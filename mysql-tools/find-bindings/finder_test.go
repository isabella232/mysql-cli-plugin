package find_bindings_test

import (
	"net/url"

	cfclient "github.com/cloudfoundry-community/go-cfclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf/mysql-cli-plugin/mysql-tools/find-bindings"
	"github.com/pivotal-cf/mysql-cli-plugin/mysql-tools/find-bindings/find-bindingsfakes"
)

var _ = Describe("BindingFinder", func() {
	Context("FindBindings", func() {
		var (
			serviceName            string
			expectedBindings       []find_bindings.Binding
			fakeCFClient           *findbindingsfakes.FakeCFClient
			service                cfclient.Service
			servicePlans           []cfclient.ServicePlan
			smallServiceInstances  []cfclient.ServiceInstance
			mediumServiceInstances []cfclient.ServiceInstance
			smallServiceBindings   []cfclient.ServiceBinding
			mediumServiceBindings  []cfclient.ServiceBinding
			smallServiceKey        []cfclient.ServiceKey
			mediumServiceKey       []cfclient.ServiceKey
			smallApp               cfclient.App
			mediumApp              cfclient.App
		)

		BeforeEach(func() {
			fakeCFClient = &findbindingsfakes.FakeCFClient{}
			serviceName = "p.mysql"

			expectedBindings = []find_bindings.Binding{
				find_bindings.Binding{
					Name:                "app1",
					ServiceInstanceName: "instance1",
					ServiceInstanceGuid: "instance1-guid",
					OrgName:             "app1-org",
					SpaceName:           "app1-space",
					Type:                "AppBinding",
				},
				find_bindings.Binding{
					Name:                "key1",
					ServiceInstanceName: "instance1",
					ServiceInstanceGuid: "instance1-guid",
					OrgName:             "app1-org",
					SpaceName:           "app1-space",
					Type:                "ServiceKeyBinding",
				},
				find_bindings.Binding{
					Name:                "app3",
					ServiceInstanceName: "instance3",
					ServiceInstanceGuid: "instance3-guid",
					OrgName:             "app3-org",
					SpaceName:           "app3-space",
					Type:                "AppBinding",
				},
				find_bindings.Binding{
					Name:                "key3",
					ServiceInstanceName: "instance3",
					ServiceInstanceGuid: "instance3-guid",
					OrgName:             "app3-org",
					SpaceName:           "app3-space",
					Type:                "ServiceKeyBinding",
				},
			}

			service = cfclient.Service{
				Label: "p.mysql",
				Guid:  "service-guid",
			}

			fakeCFClient.ListServicesByQueryReturns([]cfclient.Service{service}, nil)

			servicePlans = []cfclient.ServicePlan{
				{Name: "small", Guid: "small-guid", ServiceGuid: "service-guid"},
				{Name: "medium", Guid: "medium-guid", ServiceGuid: "service-guid"},
				{Name: "large", Guid: "large-guid", ServiceGuid: "service-guid"},
			}

			fakeCFClient.ListServicePlansByQueryReturns(servicePlans, nil)

			smallServiceInstances = []cfclient.ServiceInstance{
				{Name: "instance1", Guid: "instance1-guid", ServicePlanGuid: "small-guid", SpaceGuid: "space1-guid"},
				{Name: "instance2", Guid: "instance2-guid", ServicePlanGuid: "small-guid", SpaceGuid: "space2-guid"},
			}

			fakeCFClient.ListServiceInstancesByQueryReturnsOnCall(0, smallServiceInstances, nil)

			mediumServiceInstances = []cfclient.ServiceInstance{
				{Name: "instance3", Guid: "instance3-guid", ServicePlanGuid: "medium-guid", SpaceGuid: "space3-guid"},
			}

			fakeCFClient.ListServiceInstancesByQueryReturnsOnCall(1, mediumServiceInstances, nil)
			fakeCFClient.ListServiceInstancesByQueryReturnsOnCall(2, []cfclient.ServiceInstance{}, nil)

			smallServiceBindings = []cfclient.ServiceBinding{
				{Guid: "binding1-guid", AppGuid: "app1-guid", ServiceInstanceGuid: "instance1-guid"},
			}
			mediumServiceBindings = []cfclient.ServiceBinding{
				{Guid: "binding3-guid", AppGuid: "app3-guid", ServiceInstanceGuid: "instance3-guid"},
			}

			fakeCFClient.ListServiceBindingsByQueryReturnsOnCall(0, smallServiceBindings, nil)
			fakeCFClient.ListServiceBindingsByQueryReturnsOnCall(1, []cfclient.ServiceBinding{}, nil)
			fakeCFClient.ListServiceBindingsByQueryReturnsOnCall(2, mediumServiceBindings, nil)

			smallServiceKey = []cfclient.ServiceKey{
				{Name: "key1"},
			}

			mediumServiceKey = []cfclient.ServiceKey{
				{Name: "key3"},
			}
			fakeCFClient.ListServiceKeysByQueryReturnsOnCall(0, smallServiceKey, nil)
			fakeCFClient.ListServiceKeysByQueryReturnsOnCall(1, []cfclient.ServiceKey{}, nil)
			fakeCFClient.ListServiceKeysByQueryReturnsOnCall(2, mediumServiceKey, nil)

			smallApp = cfclient.App{
				Guid: "app1-guid",
				Name: "app1",
				SpaceData: cfclient.SpaceResource{
					Entity: cfclient.Space{
						Name:             "app1-space",
						OrganizationGuid: "app1-org-guid",
						OrgData: cfclient.OrgResource{
							Entity: cfclient.Org{
								Name: "app1-org",
							},
						},
					},
				},
			}

			mediumApp = cfclient.App{
				Guid: "app3-guid",
				Name: "app3",
				SpaceData: cfclient.SpaceResource{
					Entity: cfclient.Space{
						Name:             "app3-space",
						OrganizationGuid: "app3-org-guid",
						OrgData: cfclient.OrgResource{
							Entity: cfclient.Org{
								Name: "app3-org",
							},
						},
					},
				},
			}

			fakeCFClient.GetAppByGuidReturnsOnCall(0, smallApp, nil)
			fakeCFClient.GetAppByGuidReturnsOnCall(1, mediumApp, nil)

			fakeCFClient.GetSpaceByGuidReturnsOnCall(0, smallApp.SpaceData.Entity, nil)
			fakeCFClient.GetSpaceByGuidReturnsOnCall(1, mediumApp.SpaceData.Entity, nil)

			fakeCFClient.GetOrgByGuidReturnsOnCall(0, smallApp.SpaceData.Entity.OrgData.Entity, nil)
			fakeCFClient.GetOrgByGuidReturnsOnCall(1, mediumApp.SpaceData.Entity.OrgData.Entity, nil)
		})

		It("returns a list of applications and service keys associated with the service", func() {
			finder := find_bindings.NewBindingFinder(fakeCFClient)
			listOfBindings, err := finder.FindBindings(serviceName)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeCFClient.ListServicesByQueryCallCount()).To(Equal(1))
			query := url.Values{}
			query.Set("q", "label:p.mysql")
			Expect(fakeCFClient.ListServicesByQueryArgsForCall(0)).To(Equal(query))

			Expect(fakeCFClient.ListServicePlansByQueryCallCount()).To(Equal(1))
			query = url.Values{}
			query.Set("q", "service_guid:service-guid")
			Expect(fakeCFClient.ListServicePlansByQueryArgsForCall(0)).To(Equal(query))

			Expect(fakeCFClient.ListServiceInstancesByQueryCallCount()).To(Equal(3))
			query = url.Values{}
			query.Set("q", "service_plan_guid:small-guid")
			Expect(fakeCFClient.ListServiceInstancesByQueryArgsForCall(0)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_plan_guid:medium-guid")
			Expect(fakeCFClient.ListServiceInstancesByQueryArgsForCall(1)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_plan_guid:large-guid")
			Expect(fakeCFClient.ListServiceInstancesByQueryArgsForCall(2)).To(Equal(query))

			Expect(fakeCFClient.ListServiceBindingsByQueryCallCount()).To(Equal(3))
			query = url.Values{}
			query.Set("q", "service_instance_guid:instance1-guid")
			Expect(fakeCFClient.ListServiceBindingsByQueryArgsForCall(0)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_instance_guid:instance2-guid")
			Expect(fakeCFClient.ListServiceBindingsByQueryArgsForCall(1)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_instance_guid:instance3-guid")
			Expect(fakeCFClient.ListServiceBindingsByQueryArgsForCall(2)).To(Equal(query))

			Expect(fakeCFClient.GetAppByGuidCallCount()).To(Equal(2))
			Expect(fakeCFClient.GetAppByGuidArgsForCall(0)).To(Equal("app1-guid"))
			Expect(fakeCFClient.GetAppByGuidArgsForCall(1)).To(Equal("app3-guid"))

			Expect(fakeCFClient.ListServiceKeysByQueryCallCount()).To(Equal(3))
			query = url.Values{}
			query.Set("q", "service_instance_guid:instance1-guid")
			Expect(fakeCFClient.ListServiceKeysByQueryArgsForCall(0)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_instance_guid:instance2-guid")
			Expect(fakeCFClient.ListServiceKeysByQueryArgsForCall(1)).To(Equal(query))

			query = url.Values{}
			query.Set("q", "service_instance_guid:instance3-guid")
			Expect(fakeCFClient.ListServiceKeysByQueryArgsForCall(2)).To(Equal(query))

			Expect(fakeCFClient.GetSpaceByGuidCallCount()).To(Equal(2))
			Expect(fakeCFClient.GetSpaceByGuidArgsForCall(0)).To(Equal("space1-guid"))
			Expect(fakeCFClient.GetSpaceByGuidArgsForCall(1)).To(Equal("space3-guid"))

			Expect(fakeCFClient.GetOrgByGuidCallCount()).To(Equal(2))
			Expect(fakeCFClient.GetOrgByGuidArgsForCall(0)).To(Equal("app1-org-guid"))
			Expect(fakeCFClient.GetOrgByGuidArgsForCall(1)).To(Equal("app3-org-guid"))

			Expect(listOfBindings).To(Equal(expectedBindings))
		})
	})
})