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

package plugin

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"code.cloudfoundry.org/cli/plugin"
	"github.com/blang/semver"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/jessevdk/go-flags"
	"github.com/pivotal-cf/mysql-cli-plugin/mysql-tools/cf"
	"github.com/pivotal-cf/mysql-cli-plugin/mysql-tools/migrate"
	"github.com/pivotal-cf/mysql-cli-plugin/mysql-tools/unpack"
	"github.com/pkg/errors"
)

var (
	version = "built from source"
	gitSHA  = "unknown"
)

const (
	usage = `cf mysql-tools migrate [-h] [--no-cleanup] <source-service-instance> <p.mysql-plan-type>
   cf mysql-tools version`
	migrateUsage = `cf mysql-tools migrate [-h] [--no-cleanup] <source-service-instance> <p.mysql-plan-type>`
)

//go:generate counterfeiter . Migrator
type Migrator interface {
	CheckServiceExists(donorInstanceName string) error
	CreateAndConfigureServiceInstance(planType, serviceName string) error
	MigrateData(donorInstanceName, recipientInstanceName string, cleanup bool) error
	RenameServiceInstances(donorInstanceName, recipientInstanceName string) error
	CleanupOnError(recipientInstanceName string) error
}

type MySQLPlugin struct {
	err error
}

func (c *MySQLPlugin) Err() error {
	return c.err
}

func (c *MySQLPlugin) Run(cliConnection plugin.CliConnection, args []string) {
	if args[0] == "CLI-MESSAGE-UNINSTALL" {
		return
	}

	if len(args) < 2 {
		// Unfortunately there is no good way currently to show the usage on a plugin
		// without having `-h` added to the command line, so we hardcode it.
		fmt.Fprintln(os.Stderr, `NAME:
   mysql-tools - Plugin to migrate mysql instances

USAGE:
   cf mysql-tools migrate [-h] [--no-cleanup] <source-service-instance> <p.mysql-plan-type>
   cf mysql-tools version`)
		os.Exit(1)
		return
	}

	command := args[1]
	migrator := migrate.NewMigrator(cf.NewClient(cliConnection), unpack.NewUnpacker())

	switch command {
	default:
		c.err = errors.Errorf("unknown command '%s'", command)
	case "version":
		fmt.Printf("%s (%s)\n", version, gitSHA)
		os.Exit(0)
	// case "find-bindings":
	case "migrate":
		c.err = Migrate(migrator, args[2:])
	}
}

func (c *MySQLPlugin) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name:    "MysqlTools",
		Version: versionFromSemver(version),
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 7,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "mysql-tools",
				HelpText: "Plugin to migrate mysql instances",
				UsageDetails: plugin.Usage{
					Usage: usage,
				},
			},
		},
	}
}

func Migrate(migrator Migrator, args []string) error {
	var opts struct {
		Args struct {
			Source   string `positional-arg-name:"<source-service-instance>"`
			PlanName string `positional-arg-name:"<p.mysql-plan-type>"`
		} `positional-args:"yes" required:"yes"`
		NoCleanup bool `long:"no-cleanup" description:"don't clean up migration app and new service instance after a failed migration'"`
	}

	parser := flags.NewParser(&opts, flags.None)
	parser.Name = "cf mysql-tools migrate"
	parser.Args()
	args, err := parser.ParseArgs(args)
	if err != nil || len(args) != 0 {
		msg := fmt.Sprintf("unexpected arguments: %s", strings.Join(args, " "))
		if err != nil {
			msg = err.Error()
		}
		return errors.Errorf("Usage: %s\n\n%s", migrateUsage, msg)
	}
	donorInstanceName := opts.Args.Source
	tempRecipientInstanceName := donorInstanceName + "-new"
	destPlan := opts.Args.PlanName
	cleanup := !opts.NoCleanup

	if err := migrator.CheckServiceExists(donorInstanceName); err != nil {
		return err
	}

	log.Printf("Warning: The mysql-tools migrate command will not migrate any triggers, routines or events.")
	productName := os.Getenv("RECIPIENT_PRODUCT_NAME")
	if productName == "" {
		productName = "p.mysql"
	}

	log.Printf("Creating new service instance %q for service %s using plan %s", tempRecipientInstanceName, productName, destPlan)
	if err := migrator.CreateAndConfigureServiceInstance(destPlan, tempRecipientInstanceName); err != nil {
		if cleanup {
			migrator.CleanupOnError(tempRecipientInstanceName)
			return fmt.Errorf("error creating service instance: %v. Attempting to clean up service %s",
				err,
				tempRecipientInstanceName,
			)
		}

		return fmt.Errorf("error creating service instance: %v. Not cleaning up service %s",
			err,
			tempRecipientInstanceName,
		)
	}

	if err := migrator.MigrateData(donorInstanceName, tempRecipientInstanceName, cleanup); err != nil {
		if cleanup {
			migrator.CleanupOnError(tempRecipientInstanceName)

			return fmt.Errorf("error migrating data: %v. Attempting to clean up service %s",
				err,
				tempRecipientInstanceName,
			)
		}

		return fmt.Errorf("error migrating data: %v. Not cleaning up service %s",
			err,
			tempRecipientInstanceName,
		)
	}

	return migrator.RenameServiceInstances(donorInstanceName, tempRecipientInstanceName)
}

//go:generate counterfeiter . CFClient
type CFClient interface {
	ListServicesByQuery(url.Values) ([]cfclient.Service, error)
}

type ServiceBinding struct {
	App   string
	Key   string
	Org   string
	Space string
}

type bindingFinder struct {
	cfClient CFClient
}

func NewBindingFinder(cfClient CFClient) *bindingFinder {
	return &bindingFinder{
		cfClient: cfClient,
	}
}

func (bf *bindingFinder) FindBindings(serviceLabel string) ([]ServiceBinding, error) {
	//cf curl " /v2/services?q=label:p.mysql"
	//	get resources entity.service_plans_url "/v2/services/9cbbd018-236f-4171-8585-594ebfde52f2/service_plans"
	//	cf curl service_plans_url
	//		get resources entity.service_instances_url "/v2/spaces/8b892a65-bf0e-4276-ad47-30757c4f2251/service_instances"
	//		cf curl service_instances_url
	//			get resources entity.service_bindings_url "/v2/service_instances/00d4ce31-bbbe-48f6-b15d-fcbd3380f50a/service_bindings"
	//			get resources entity.service_keys_url "/v2/service_instances/00d4ce31-bbbe-48f6-b15d-fcbd3380f50a/service_keys"
	//			cf curl service_bindings_url
	//				get resources entity.app_guid
	//			cf curl service_keys_url
	//				get resources entity.name    ?
	//				get resources metadata.guid  ?
	//			get resources entity.space_url "/v2/spaces/8b892a65-bf0e-4276-ad47-30757c4f2251"
	//			cf curl space_url
	//				get resources entity.name
	//				get resources entity.organization_url "/v2/organizations/10b9207b-1c15-46d8-9946-a2374b8c40e5"
	//				cf curl organization_url
	//					get resources entity.name
	u := url.Values{}
	u.Set("q", fmt.Sprintf("label:%s", serviceLabel))
	services, _ := bf.cfClient.ListServicesByQuery(u)
	return []ServiceBinding{
		{App: "app", Key: "", Org: "org-name", Space: "space-name"},
	}, nil
}

func versionFromSemver(in string) plugin.VersionType {
	var unknownVersion = plugin.VersionType{
		Major: 0,
		Minor: 0,
		Build: 1,
	}

	if in == "built from source" {
		return unknownVersion
	}

	v, err := semver.Parse(in)
	if err != nil {
		return unknownVersion
	}

	return plugin.VersionType{
		Major: int(v.Major),
		Minor: int(v.Minor),
		Build: int(v.Patch),
	}
}
