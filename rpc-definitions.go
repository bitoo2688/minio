/*
 * Minio Cloud Storage, (C) 2014 Minio, Inc.
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

type Ifc struct {
	IP   string `json:"ip"`
	Mask string `json:"mask"`
	Eth  string `json:"ifc"`
}

type ServerArg struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	ID   string `json:"id"`
}

type ServerRep struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	ID   string `json:"id"`
}

type DefaultRep struct {
	Error   int64  `json:"error"`
	Message string `json:"message"`
}

type ServerListRep struct {
	List []ServerRep
}

type DiskStatsRep struct {
	Disks []string
}

type MemStatsRep struct {
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

type NetStatsRep struct {
	Interfaces []Ifc
}

type SysInfoRep struct {
	Hostname  string `json:"hostname"`
	SysARCH   string `json:"sys.arch"`
	SysOS     string `json:"sys.os"`
	SysCPUS   int    `json:"sys.ncpus"`
	Routines  int    `json:"goroutines"`
	GOVersion string `json:"goversion"`
}

type ListRep struct {
	List []ServerArg `json:"list"`
}
