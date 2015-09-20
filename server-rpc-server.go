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
	"net/http"
	"os"
	"runtime"

	"github.com/minio/minio/pkg/probe"
)

type serverServerService struct{}

func (this *serverServerService) Add(r *http.Request, arg *ServerArg, rep *DefaultRep) error {
	rep.Error = 0
	rep.Message = "Added successfully"
	return nil
}

func (this *serverServerService) MemStats(r *http.Request, arg *ServerArg, rep *MemStatsRep) error {
	rep.Total = 64 * 1024 * 1024 * 1024
	rep.Free = 9 * 1024 * 1024 * 1024
	return nil
}

func (this *serverServerService) DiskStats(r *http.Request, arg *ServerArg, rep *DiskStatsRep) error {
	rep.Disks = []string{"/mnt/disk1", "/mnt/disk2", "/mnt/disk3", "/mnt/disk4", "/mnt/disk5", "/mnt/disk6"}
	return nil
}

func (this *serverServerService) SysInfo(r *http.Request, arg *ServerArg, rep *SysInfoRep) error {
	rep.SysARCH = runtime.GOARCH
	rep.SysOS = runtime.GOOS
	rep.SysCPUS = runtime.NumCPU()
	rep.Routines = runtime.NumGoroutine()
	rep.GOVersion = runtime.Version()
	var err error
	rep.Hostname, err = os.Hostname()
	if err != nil {
		return probe.WrapError(probe.NewError(err))
	}
	return nil
}

func (this *serverServerService) NetStats(r *http.Request, arg *ServerArg, rep *NetStatsRep) error {
	rep.Interfaces = []Ifc{{"192.168.1.1", "255.255.255.0", "eth0"}}
	return nil
}

func NewserverServerService() *serverServerService {
	s := &serverServerService{}
	return s
}
