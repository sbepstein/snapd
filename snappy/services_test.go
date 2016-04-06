// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package snappy

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"

	. "gopkg.in/check.v1"

	"github.com/ubuntu-core/snappy/arch"
	"github.com/ubuntu-core/snappy/dirs"
	"github.com/ubuntu-core/snappy/systemd"
	"github.com/ubuntu-core/snappy/timeout"
)

func (s *SnapTestSuite) TestAddPackageServicesStripsGlobalRootdir(c *C) {
	// ensure that even with a global rootdir the paths in the generated
	// .services file are setup correctly (i.e. that the global root
	// is stripped)
	c.Assert(dirs.GlobalRootDir, Not(Equals), "/")

	yamlFile, err := makeInstalledMockSnap(s.tempdir, "")
	c.Assert(err, IsNil)
	m, err := parseSnapYamlFile(yamlFile)
	c.Assert(err, IsNil)
	baseDir := filepath.Dir(filepath.Dir(yamlFile))
	err = addPackageServices(m, baseDir, false, nil)
	c.Assert(err, IsNil)

	content, err := ioutil.ReadFile(filepath.Join(s.tempdir, "/etc/systemd/system/hello-snap_svc1_1.10.service"))
	c.Assert(err, IsNil)

	baseDirWithoutRootPrefix := "/snaps/" + helloSnapComposedName + "/1.10"
	verbs := []string{"Start", "Stop", "StopPost"}
	bins := []string{"hello", "goodbye", "missya"}
	for i := range verbs {
		expected := fmt.Sprintf("Exec%s=/usr/bin/ubuntu-core-launcher hello-snap.svc1 %s_svc1_1.10 %s/bin/%s", verbs[i], helloSnapComposedName, baseDirWithoutRootPrefix, bins[i])
		c.Check(string(content), Matches, "(?ms).*^"+regexp.QuoteMeta(expected)) // check.v1 adds ^ and $ around the regexp provided
	}
}

func (s *SnapTestSuite) TestAddPackageBinariesStripsGlobalRootdir(c *C) {
	// ensure that even with a global rootdir the paths in the generated
	// .services file are setup correctly (i.e. that the global root
	// is stripped)
	c.Assert(dirs.GlobalRootDir, Not(Equals), "/")

	yamlFile, err := makeInstalledMockSnap(s.tempdir, "")
	c.Assert(err, IsNil)
	m, err := parseSnapYamlFile(yamlFile)
	c.Assert(err, IsNil)
	baseDir := filepath.Dir(filepath.Dir(yamlFile))
	err = addPackageBinaries(m, baseDir)
	c.Assert(err, IsNil)

	content, err := ioutil.ReadFile(filepath.Join(s.tempdir, "/snaps/bin/hello-snap.hello"))
	c.Assert(err, IsNil)

	needle := `
ubuntu-core-launcher hello-snap.hello hello-snap_hello_1.10 /snaps/hello-snap/1.10/bin/hello "$@"
`
	c.Assert(string(content), Matches, "(?ms).*"+regexp.QuoteMeta(needle)+".*")
}

var (
	expectedServiceWrapperFmt = `[Unit]
Description=A fun webserver
%s
X-Snappy=yes

[Service]
ExecStart=/usr/bin/ubuntu-core-launcher xkcd-webserver.xkcd-webserver xkcd-webserver_xkcd-webserver_0.3.4 /snaps/xkcd-webserver/0.3.4/bin/foo start
Restart=on-failure
WorkingDirectory=/var/lib/snaps/xkcd-webserver/0.3.4/
Environment="SNAP_APP=xkcd-webserver_xkcd-webserver_0.3.4" "SNAP=/snaps/xkcd-webserver/0.3.4/" "SNAP_DATA=/var/lib/snaps/xkcd-webserver/0.3.4/" "SNAP_NAME=xkcd-webserver" "SNAP_VERSION=0.3.4" "SNAP_ARCH=%[3]s" "SNAP_USER_DATA=/root/snaps/xkcd-webserver/0.3.4/" "SNAP_APP_PATH=/snaps/xkcd-webserver/0.3.4/" "SNAP_APP_DATA_PATH=/var/lib/snaps/xkcd-webserver/0.3.4/" "SNAP_APP_USER_DATA_PATH=/root/snaps/xkcd-webserver/0.3.4/"
ExecStop=/usr/bin/ubuntu-core-launcher xkcd-webserver.xkcd-webserver xkcd-webserver_xkcd-webserver_0.3.4 /snaps/xkcd-webserver/0.3.4/bin/foo stop
ExecStopPost=/usr/bin/ubuntu-core-launcher xkcd-webserver.xkcd-webserver xkcd-webserver_xkcd-webserver_0.3.4 /snaps/xkcd-webserver/0.3.4/bin/foo post-stop
TimeoutStopSec=30
%[2]s

[Install]
WantedBy=multi-user.target
`
	expectedServiceAppWrapper     = fmt.Sprintf(expectedServiceWrapperFmt, "After=ubuntu-snappy.frameworks.target\nRequires=ubuntu-snappy.frameworks.target", "Type=simple\n", arch.UbuntuArchitecture())
	expectedServiceFmkWrapper     = fmt.Sprintf(expectedServiceWrapperFmt, "Before=ubuntu-snappy.frameworks.target\nAfter=ubuntu-snappy.frameworks-pre.target\nRequires=ubuntu-snappy.frameworks-pre.target", "Type=dbus\nBusName=foo.bar.baz", arch.UbuntuArchitecture())
	expectedSocketUsingWrapper    = fmt.Sprintf(expectedServiceWrapperFmt, "After=ubuntu-snappy.frameworks.target xkcd-webserver_xkcd-webserver_0.3.4.socket\nRequires=ubuntu-snappy.frameworks.target xkcd-webserver_xkcd-webserver_0.3.4.socket", "Type=simple\n", arch.UbuntuArchitecture())
	expectedTypeForkingFmkWrapper = fmt.Sprintf(expectedServiceWrapperFmt, "After=ubuntu-snappy.frameworks.target\nRequires=ubuntu-snappy.frameworks.target", "Type=forking\n", arch.UbuntuArchitecture())
)

func (s *SnapTestSuite) TestSnappyGenerateSnapServiceTypeForking(c *C) {
	service := &AppYaml{
		Name:        "xkcd-webserver",
		Command:     "bin/foo start",
		Stop:        "bin/foo stop",
		PostStop:    "bin/foo post-stop",
		StopTimeout: timeout.DefaultTimeout,
		Description: "A fun webserver",
		Daemon:      "forking",
	}
	pkgPath := "/snaps/xkcd-webserver/0.3.4/"
	aaProfile := "xkcd-webserver_xkcd-webserver_0.3.4"
	m := snapYaml{Name: "xkcd-webserver",
		Version: "0.3.4"}

	generatedWrapper, err := generateSnapServicesFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, IsNil)
	c.Assert(generatedWrapper, Equals, expectedTypeForkingFmkWrapper)
}

func (s *SnapTestSuite) TestSnappyGenerateSnapServiceAppWrapper(c *C) {
	service := &AppYaml{
		Name:        "xkcd-webserver",
		Command:     "bin/foo start",
		Stop:        "bin/foo stop",
		PostStop:    "bin/foo post-stop",
		StopTimeout: timeout.DefaultTimeout,
		Description: "A fun webserver",
		Daemon:      "simple",
	}
	pkgPath := "/snaps/xkcd-webserver/0.3.4/"
	aaProfile := "xkcd-webserver_xkcd-webserver_0.3.4"
	m := snapYaml{Name: "xkcd-webserver",
		Version: "0.3.4"}

	generatedWrapper, err := generateSnapServicesFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, IsNil)
	c.Assert(generatedWrapper, Equals, expectedServiceAppWrapper)
}

func (s *SnapTestSuite) TestSnappyGenerateSnapServiceRestart(c *C) {
	service := &AppYaml{
		Name:        "xkcd-webserver",
		RestartCond: systemd.RestartOnAbort,
		Daemon:      "simple",
	}
	pkgPath := "/snaps/xkcd-webserver/0.3.4/"
	aaProfile := "xkcd-webserver_xkcd-webserver_0.3.4"
	m := snapYaml{
		Name:    "xkcd-webserver",
		Version: "0.3.4",
	}

	generatedWrapper, err := generateSnapServicesFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, IsNil)
	c.Assert(generatedWrapper, Matches, `(?ms).*^Restart=on-abort$.*`)
}

func (s *SnapTestSuite) TestSnappyGenerateSnapServiceWrapperWhitelist(c *C) {
	service := &AppYaml{Name: "xkcd-webserver",
		Command:     "bin/foo start",
		Stop:        "bin/foo stop",
		PostStop:    "bin/foo post-stop",
		StopTimeout: timeout.DefaultTimeout,
		Description: "A fun webserver\nExec=foo",
		Daemon:      "simple",
	}
	pkgPath := "/snaps/xkcd-webserver.canonical/0.3.4/"
	aaProfile := "xkcd-webserver.canonical_xkcd-webserver_0.3.4"
	m := snapYaml{Name: "xkcd-webserver",
		Version: "0.3.4"}

	_, err := generateSnapServicesFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, NotNil)
}

func (s *SnapTestSuite) TestRemovePackageServiceKills(c *C) {
	c.Skip("needs porting to new squashfs based snap activation!")

	// make Stop not work
	var sysdLog [][]string
	systemd.SystemctlCmd = func(cmd ...string) ([]byte, error) {
		sysdLog = append(sysdLog, cmd)
		return []byte("ActiveState=active\n"), nil
	}
	yamlFile, err := makeInstalledMockSnap(s.tempdir, `name: wat
version: 42
apps:
 wat:
   command: wat
   stop-timeout: 25
   daemon: forking
`)
	c.Assert(err, IsNil)
	m, err := parseSnapYamlFile(yamlFile)
	c.Assert(err, IsNil)
	inter := &MockProgressMeter{}
	c.Check(removePackageServices(m, filepath.Dir(filepath.Dir(yamlFile)), inter), IsNil)
	c.Assert(len(inter.notified) > 0, Equals, true)
	c.Check(inter.notified[len(inter.notified)-1], Equals, "wat_wat_42.service refused to stop, killing.")
	c.Assert(len(sysdLog) >= 3, Equals, true)
	sd1 := sysdLog[len(sysdLog)-3]
	sd2 := sysdLog[len(sysdLog)-2]
	c.Check(sd1, DeepEquals, []string{"kill", "wat_wat_42.service", "-s", "TERM"})
	c.Check(sd2, DeepEquals, []string{"kill", "wat_wat_42.service", "-s", "KILL"})
}

func (s *SnapTestSuite) TestSnappyGenerateSnapSocket(c *C) {
	service := &AppYaml{Name: "xkcd-webserver",
		Command:      "bin/foo start",
		Description:  "meep",
		Socket:       true,
		ListenStream: "/var/run/docker.sock",
		SocketMode:   "0660",
		Daemon:       "simple",
	}
	pkgPath := "/snaps/xkcd-webserver.canonical/0.3.4/"
	aaProfile := "xkcd-webserver.canonical_xkcd-webserver_0.3.4"
	m := snapYaml{
		Name:    "xkcd-webserver",
		Version: "0.3.4"}

	content, err := generateSnapSocketFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, IsNil)
	c.Assert(content, Equals, `[Unit]
Description= Socket Unit File
PartOf=xkcd-webserver_xkcd-webserver_0.3.4.service
X-Snappy=yes

[Socket]
ListenStream=/var/run/docker.sock
SocketMode=0660

[Install]
WantedBy=sockets.target
`)
}

func (s *SnapTestSuite) TestSnappyGenerateSnapServiceWithSocket(c *C) {
	service := &AppYaml{
		Name:        "xkcd-webserver",
		Command:     "bin/foo start",
		Stop:        "bin/foo stop",
		PostStop:    "bin/foo post-stop",
		StopTimeout: timeout.DefaultTimeout,
		Description: "A fun webserver",
		Socket:      true,
		Daemon:      "simple",
	}
	pkgPath := "/snaps/xkcd-webserver/0.3.4/"
	aaProfile := "xkcd-webserver_xkcd-webserver_0.3.4"
	m := snapYaml{Name: "xkcd-webserver",
		Version: "0.3.4"}

	generatedWrapper, err := generateSnapServicesFile(service, pkgPath, aaProfile, &m)
	c.Assert(err, IsNil)
	c.Assert(generatedWrapper, Equals, expectedSocketUsingWrapper)
}

func (s *SnapTestSuite) TestGenerateSnapSocketFile(c *C) {
	srv := &AppYaml{}
	baseDir := "/base/dir"
	aaProfile := "pkg_app_1.0"
	m := &snapYaml{}

	// no socket mode means 0660
	content, err := generateSnapSocketFile(srv, baseDir, aaProfile, m)
	c.Assert(err, IsNil)
	c.Assert(content, Matches, "(?ms).*SocketMode=0660")

	// SocketMode itself is honored
	srv.SocketMode = "0600"
	content, err = generateSnapSocketFile(srv, baseDir, aaProfile, m)
	c.Assert(err, IsNil)
	c.Assert(content, Matches, "(?ms).*SocketMode=0600")

}