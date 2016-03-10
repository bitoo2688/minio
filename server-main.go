/*
 * Minio Cloud Storage, (C) 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/minio/cli"
	"github.com/minio/minio/pkg/minhttp"
	"github.com/minio/minio/pkg/probe"
)

var serverCmd = cli.Command{
	Name:  "server",
	Usage: "Start Minio cloud storage server.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "min-free-disk, M",
			Value: "5%",
		},
	},
	Action: serverMain,
	CustomHelpTemplate: `NAME:
  minio {{.Name}} - {{.Usage}}

USAGE:
  minio {{.Name}} [OPTION VALUE] PATH

OPTIONS:
  {{range .Flags}}{{.}}
  {{end}}

ENVIRONMENT VARIABLES:
  MINIO_ACCESS_KEY, MINIO_SECRET_KEY: Access and secret key to use.

EXAMPLES:
  1. Start minio server on Linux.
      $ minio {{.Name}} /home/shared

  2. Start minio server on Windows.
      $ minio {{.Name}} C:\MyShare

  3. Start minio server bound to a specific IP:PORT, when you have multiple network interfaces.
      $ minio --address 192.168.1.101:9000 {{.Name}} /home/shared

  4. Start minio server with minimum free disk threshold to 5%
      $ minio {{.Name}} --min-free-disk 5% /home/shared/Pictures

`,
}

// cloudServerConfig - http server config
type cloudServerConfig struct {
	/// HTTP server options
	Address   string // Address:Port listening
	AccessLog bool   // Enable access log handler

	// Credentials.
	AccessKeyID     string // Access key id.
	SecretAccessKey string // Secret access key.
	Region          string // Region string.

	/// FS options
	Path        string // Path to export for cloud storage
	MinFreeDisk int64  // Minimum free disk space for filesystem
	MaxBuckets  int    // Maximum number of buckets suppported by filesystem.

	/// TLS service
	TLS      bool   // TLS on when certs are specified
	CertFile string // Domain certificate
	KeyFile  string // Domain key
}

// configureServer configure a new server instance
func configureServer(conf cloudServerConfig) (*http.Server, *probe.Error) {
	// Minio server config
	apiServer := &http.Server{
		Addr:           conf.Address,
		Handler:        serverHandler(conf),
		MaxHeaderBytes: 1 << 20,
	}

	if conf.TLS {
		var err error
		apiServer.TLSConfig = &tls.Config{}
		apiServer.TLSConfig.Certificates = make([]tls.Certificate, 1)
		apiServer.TLSConfig.Certificates[0], err = tls.LoadX509KeyPair(conf.CertFile, conf.KeyFile)
		if err != nil {
			return nil, probe.NewError(err)
		}
	}
	return apiServer, nil
}

func printServerMsg(serverConf *http.Server) {
	host, port, e := net.SplitHostPort(serverConf.Addr)
	fatalIf(probe.NewError(e), "Unable to split host port.", nil)

	var hosts []string
	switch {
	case host != "":
		hosts = append(hosts, host)
	default:
		addrs, e := net.InterfaceAddrs()
		fatalIf(probe.NewError(e), "Unable to get interface address.", nil)

		for _, addr := range addrs {
			if addr.Network() == "ip+net" {
				host := strings.Split(addr.String(), "/")[0]
				if ip := net.ParseIP(host); ip.To4() != nil {
					hosts = append(hosts, host)
				}
			}
		}
	}
	for _, host := range hosts {
		if serverConf.TLSConfig != nil {
			Printf("    https://%s:%s\n", host, port)
		} else {
			Printf("    http://%s:%s\n", host, port)
		}
	}
}

// parse input string with percent to int64
func parsePercentToInt(s string, bitSize int) (int64, *probe.Error) {
	i := strings.Index(s, "%")
	if i < 0 {
		// no percentage string found try to parse the whole string anyways
		p, e := strconv.ParseInt(s, 10, bitSize)
		if e != nil {
			return 0, probe.NewError(e)
		}
		return p, nil
	}
	p, e := strconv.ParseInt(s[:i], 10, bitSize)
	if e != nil {
		return 0, probe.NewError(e)
	}
	return p, nil
}
func setLogger(conf *configV2) *probe.Error {
	if conf.IsMongoLoggingEnabled() {
		if err := log2Mongo(conf.MongoLogger.Addr, conf.MongoLogger.DB, conf.MongoLogger.Collection); err != nil {
			return err.Trace(conf.MongoLogger.Addr, conf.MongoLogger.DB, conf.MongoLogger.Collection)
		}
	}
	if conf.IsSysloggingEnabled() {
		if err := log2Syslog(conf.SyslogLogger.Network, conf.SyslogLogger.Addr); err != nil {
			return err.Trace(conf.SyslogLogger.Network, conf.SyslogLogger.Addr)
		}
	}
	if conf.IsFileLoggingEnabled() {
		if err := log2File(conf.FileLogger.Filename); err != nil {
			return err.Trace(conf.FileLogger.Filename)
		}
	}
	return nil
}

// Generates config if it doesn't exist, otherwise returns back the saved ones.
func getConfig() (*configV2, *probe.Error) {
	if err := createConfigPath(); err != nil {
		return nil, err.Trace()
	}
	config, err := loadConfigV2()
	if err != nil {
		if os.IsNotExist(err.ToGoError()) {
			// Initialize new config, since config file doesn't exist yet
			config = &configV2{}
			config.Version = "2"
			config.Credentials.AccessKeyID = string(mustGenerateAccessKeyID())
			config.Credentials.SecretAccessKey = string(mustGenerateSecretAccessKey())
			config.Credentials.Region = "us-east-1"
			if err = saveConfig(config); err != nil {
				return nil, err.Trace()
			}
			return config, nil
		}
		return nil, err.Trace()
	}
	return config, nil
}

type accessKeys struct {
	*configV2
}

func (a accessKeys) String() string {
	magenta := color.New(color.FgMagenta, color.Bold).SprintFunc()
	white := color.New(color.FgWhite, color.Bold).SprintfFunc()
	return fmt.Sprint(magenta("AccessKey: ") + white(a.Credentials.AccessKeyID) + "  " + magenta("SecretKey: ") + white(a.Credentials.SecretAccessKey) + "  " + magenta("Region: ") + white(a.Credentials.Region))
}

// JSON - json formatted output
func (a accessKeys) JSON() string {
	b, e := json.Marshal(a)
	errorIf(probe.NewError(e), "Unable to marshal json", nil)
	return string(b)
}

// initServer initialize server
func initServer() (*configV2, *probe.Error) {
	conf, err := getConfig()
	if err != nil {
		return nil, err.Trace()
	}
	if err := setLogger(conf); err != nil {
		return nil, err.Trace()
	}
	if conf != nil {
		Println()
		Println(accessKeys{conf})
	}
	return conf, nil
}

func checkServerSyntax(c *cli.Context) {
	if !c.Args().Present() || c.Args().First() == "help" {
		cli.ShowCommandHelpAndExit(c, "server", 1)
	}
	if len(c.Args()) > 1 {
		fatalIf(probe.NewError(errInvalidArgument), "Unnecessary arguments passed. Please refer ‘mc server help’", nil)
	}
	path := strings.TrimSpace(c.Args().Last())
	if path == "" {
		fatalIf(probe.NewError(errInvalidArgument), "Path argument cannot be empty.", nil)
	}
}

// extract port number from address.
// address should be of the form host:port
func getPort(address string) int {
	_, portStr, e := net.SplitHostPort(address)
	fatalIf(probe.NewError(e), "Unable to split host port.", nil)
	portInt, e := strconv.Atoi(portStr)
	fatalIf(probe.NewError(e), "Invalid port number.", nil)
	return portInt
}

// Make sure that none of the other processes are listening on the
// specified port on any of the interfaces.
//
// On linux if a process is listening on 127.0.0.1:9000 then Listen()
// on ":9000" fails with the error "port already in use".
// However on Mac OSX Listen() on ":9000" falls back to the IPv6 address.
// This causes confusion on Mac OSX that minio server is not reachable
// on 127.0.0.1 even though minio server is running. So before we start
// the minio server we make sure that the port is free on all the IPs.
func checkPortAvailability(port int) {
	ifcs, e := net.Interfaces()
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to list interfaces.", nil)
	}
	for _, ifc := range ifcs {
		addrs, e := ifc.Addrs()
		if e != nil {
			fatalIf(probe.NewError(e), fmt.Sprintf("Unable to list addresses on interface %s.", ifc.Name), nil)
		}
		for _, addr := range addrs {
			ip := addr.(*net.IPNet).IP
			network := "tcp4"
			if ip.To4() == nil {
				network = "tcp6"
			}
			tcpAddr := net.TCPAddr{IP: ip, Port: port, Zone: ifc.Name}
			l, e := net.ListenTCP(network, &tcpAddr)
			if e != nil {
				fatalIf(probe.NewError(e), fmt.Sprintf("Unable to listen on IP %s, port %.d", tcpAddr.IP, tcpAddr.Port), nil)
			}
			e = l.Close()
			if e != nil {
				fatalIf(probe.NewError(e), fmt.Sprintf("Unable to close listener on IP %s, port %.d", tcpAddr.IP, tcpAddr.Port), nil)
			}
		}
	}
}

func serverMain(c *cli.Context) {
	checkServerSyntax(c)
	address := c.GlobalString("address")
	checkPortAvailability(getPort(address))

	conf, err := initServer()
	fatalIf(err.Trace(), "Failed to read config for minio.", nil)

	certFile := c.GlobalString("cert")
	keyFile := c.GlobalString("key")
	if (certFile != "" && keyFile == "") || (certFile == "" && keyFile != "") {
		fatalIf(probe.NewError(errInvalidArgument), "Both certificate and key are required to enable https.", nil)
	}

	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if accessKey != "" && secretKey != "" {
		if !isValidAccessKey(accessKey) {
			fatalIf(probe.NewError(errInvalidArgument), "Access key does not have required length", nil)
		}
		if !isValidSecretKey(secretKey) {
			fatalIf(probe.NewError(errInvalidArgument), "Secret key does not have required length", nil)
		}

		conf.Credentials.AccessKeyID = accessKey
		conf.Credentials.SecretAccessKey = secretKey

		err = saveConfig(conf)
		fatalIf(err.Trace(), "Unable to save credentials to config.", nil)
	}

	minFreeDisk, err := parsePercentToInt(c.String("min-free-disk"), 64)
	fatalIf(err.Trace(c.String("min-free-disk")), "Invalid minium free disk size "+c.String("min-free-disk")+" passed.", nil)

	path := strings.TrimSpace(c.Args().Last())
	// Last argument is always path
	if _, err := os.Stat(path); err != nil {
		fatalIf(probe.NewError(err), "Unable to validate the path", nil)
	}
	region := conf.Credentials.Region
	if region == "" {
		region = "us-east-1"
	}
	tls := (certFile != "" && keyFile != "")
	serverConfig := cloudServerConfig{
		Address:         address,
		AccessLog:       c.GlobalBool("enable-accesslog"),
		AccessKeyID:     conf.Credentials.AccessKeyID,
		SecretAccessKey: conf.Credentials.SecretAccessKey,
		Region:          region,
		Path:            path,
		MinFreeDisk:     minFreeDisk,
		TLS:             tls,
		CertFile:        certFile,
		KeyFile:         keyFile,
	}

	// configure server.
	apiServer, err := configureServer(serverConfig)
	errorIf(err.Trace(), "Failed to configure API server.", nil)

	Println("\nMinio Object Storage:")
	printServerMsg(apiServer)

	Println("\nMinio Browser:")
	printServerMsg(apiServer)

	Println("\nTo configure Minio Client:")
	if runtime.GOOS == "windows" {
		Println("    Download \"mc\" from https://dl.minio.io/client/mc/release/" + runtime.GOOS + "-" + runtime.GOARCH + "/mc.exe")
		Println("    $ mc.exe config host add myminio http://localhost:9000 " + conf.Credentials.AccessKeyID + " " + conf.Credentials.SecretAccessKey)
	} else {
		Println("    $ wget https://dl.minio.io/client/mc/release/" + runtime.GOOS + "-" + runtime.GOARCH + "/mc")
		Println("    $ chmod 755 mc")
		Println("    $ ./mc config host add myminio http://localhost:9000 " + conf.Credentials.AccessKeyID + " " + conf.Credentials.SecretAccessKey)
	}

	// Start server.
	err = minhttp.ListenAndServe(apiServer)
	errorIf(err.Trace(), "Failed to start the minio server.", nil)
}
