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

package migrate

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

//go:generate counterfeiter . client
type client interface {
	ServiceExists(serviceName string) bool
	CreateServiceInstance(planType, instanceName string) error
	GetHostnames(instanceName string) ([]string, error)
	UpdateServiceConfig(instanceName string, jsonParams string) error
	BindService(appName, serviceName string) error
	DeleteApp(appName string) error
	DeleteServiceInstance(instanceName string) error
	DumpLogs(appName string)
	PushApp(path, appName string) error
	RenameService(oldName, newName string) error
	RunTask(appName, command string) error
	StartApp(appName string) error
}

type unpacker interface {
	Unpack(destDir string) error
}

func NewMigrator(client client, unpacker unpacker) *Migrator {
	return &Migrator{
		client:   client,
		unpacker: unpacker,
	}
}

type Migrator struct {
	appName  string
	client   client
	unpacker unpacker
}

func (m *Migrator) CheckServiceExists(donorInstanceName string) error {
	if ! m.client.ServiceExists(donorInstanceName) {
		return fmt.Errorf("Service instance %s not found", donorInstanceName)
	}

	return nil
}

func (m *Migrator) CreateAndConfigureServiceInstance(planType, serviceName string) error {
	if err := m.client.CreateServiceInstance(planType, serviceName); err != nil {
		return errors.Wrap(err, "Error creating service instance")
	}

	instanceIP, err := m.client.GetHostnames(serviceName)
	if err != nil {
		m.client.DeleteServiceInstance(serviceName)
		return errors.Wrap(err, "Error obtaining hostname for new service instance")
	}

	err = m.client.UpdateServiceConfig(serviceName,
		fmt.Sprintf(`{"enable_tls": ["%s"]}`, instanceIP))
	return nil
}

func (m *Migrator) MigrateData(donorInstanceName, recipientInstanceName string) error {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "migrate_app_")

	if err != nil {

		return errors.Errorf("Error creating temp directory: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("Unpacking assets for migration to %s", tmpDir)
	if err = m.unpacker.Unpack(tmpDir); err != nil {
		return errors.Errorf("Error extracting migrate assets: %s", err)
	}

	log.Print("Started to push app")
	m.appName = "migrate-app-" + uuid.New()
	if err = m.client.PushApp(tmpDir, m.appName); err != nil {
		return errors.Errorf("failed to push application: %s", err)
	}
	defer func() {
		m.client.DeleteApp(m.appName)
		log.Print("Cleaning up...")
	}()
	log.Print("Successfully pushed app")

	if err = m.client.BindService(m.appName, donorInstanceName); err != nil {
		return errors.Errorf("failed to bind-service %q to application %q: %s", m.appName, donorInstanceName, err)
	}
	log.Print("Successfully bound app to v1 instance")

	if err = m.client.BindService(m.appName, recipientInstanceName); err != nil {
		return errors.Errorf("failed to bind-service %q to application %q: %s", m.appName, recipientInstanceName, err)
	}
	log.Print("Successfully bound app to v2 instance")

	log.Print("Starting migration app")
	if err = m.client.StartApp(m.appName); err != nil {
		return errors.Errorf("failed to start application %q: %s", m.appName, err)
	}

	log.Print("Started to run migration task")
	command := fmt.Sprintf("./migrate %s %s", donorInstanceName, recipientInstanceName)
	if err = m.client.RunTask(m.appName, command); err != nil {
		log.Printf("Migration failed: %s", err)
		log.Print("Fetching log output...")
		time.Sleep(5 * time.Second)
		m.client.DumpLogs(m.appName)
		return err
	}

	log.Print("Migration completed successfully")

	return nil
}

func (m *Migrator) RenameServiceInstances(donorInstanceName, recipientInstanceName string) error {
	newDonorInstanceName := donorInstanceName + "-old"
	if err := m.client.RenameService(donorInstanceName, newDonorInstanceName); err != nil {
		return fmt.Errorf("Error renaming service instance %s: %s", donorInstanceName, err)
	}

	if err := m.client.RenameService(recipientInstanceName, donorInstanceName); err != nil {
		return fmt.Errorf("Error renaming service instance %s: %s", recipientInstanceName, err)
	}

	return nil
}

func (m *Migrator) CleanupOnError(recipientServiceInstance string) error {
	return m.client.DeleteServiceInstance(recipientServiceInstance)
}