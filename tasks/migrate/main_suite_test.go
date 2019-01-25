// Copyright (C) 2018-Present Pivotal Software, Inc. All rights reserved.
//
// This program and the accompanying materials are made available under the terms of the under the Apache License,
// Version 2.0 (the "License”); you may not use this file except in compliance with the License. You may obtain a copy
// of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.

package main_test

import (
	"log"
	"testing"

	"github.com/fsouza/go-dockerclient"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	. "github.com/pivotal/mysql-test-utils/dockertest"
)

func TestMigrate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migrate Suite")
}

const (
	mysqlDockerImage             = "percona:5.7"
	mySQLDockerPort  docker.Port = "3306/tcp"
)

var (
	migrateTaskBinPath string
	dockerClient       *docker.Client
	sessionID          string
)

var _ = BeforeSuite(func() {
	log.SetOutput(GinkgoWriter)
	_ = mysql.SetLogger(log.New(GinkgoWriter, "[mysql] ", log.Ldate|log.Ltime|log.Lshortfile))

	var err error
	dockerClient, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())
	Expect(PullImage(dockerClient, mysqlDockerImage)).To(Succeed())

	migrateTaskBinPath, err = gexec.Build("github.com/pivotal-cf/mysql-cli-plugin/tasks/migrate")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

var _ = BeforeEach(func() {
	sessionID = uuid.New().String()
})

func createMySQLContainer(name string) (*docker.Container, error) {
	return RunContainer(
		dockerClient,
		name+"."+sessionID,
		AddExposedPorts(mySQLDockerPort),
		WithImage(mysqlDockerImage),
		AddEnvVars(
			"MYSQL_ALLOW_EMPTY_PASSWORD=1",
		),
	)
}
