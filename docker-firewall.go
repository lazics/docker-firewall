package main

import "context"
import "fmt"
import "strings"
import "net"
import "sort"

import "github.com/docker/docker/api/types"
import "github.com/docker/docker/api/types/events"
import "github.com/docker/docker/api/types/filters"
import "github.com/docker/docker/client"

// DockerClient :
type DockerClient struct {
	ctx          context.Context
	dockerClient *client.Client

	eventMessageChannel <-chan events.Message
	eventErrorChannel   <-chan error
}

// Connect :
func (dockerClient *DockerClient) Connect() error {
	if dockerClient.dockerClient == nil {
		dockerClient.ctx = context.Background()
		if _dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation()); err == nil && _dockerClient != nil {
			dockerClient.dockerClient = _dockerClient
		} else {
			return err
		}
	}
	return nil
}

// Close :
func (dockerClient *DockerClient) Close() {
	if dockerClient.dockerClient != nil {
		dockerClient.dockerClient.Close()
		dockerClient.dockerClient = nil
	}
}

// MonitorEvents :
func (dockerClient *DockerClient) MonitorEvents(filters filters.Args) {
	dockerClient.eventMessageChannel, dockerClient.eventErrorChannel = dockerClient.dockerClient.Events(dockerClient.ctx, types.EventsOptions{
		Filters: filters,
	})
}

// MonitorNetworkEvents :
func (dockerClient *DockerClient) MonitorNetworkEvents() {
	filters := filters.NewArgs()
	filters.Add("type", events.NetworkEventType)
	dockerClient.MonitorEvents(filters)
}

// Rules :
type Rules []string

// Append :
func (Rules *Rules) Append(s string) {
	*Rules = append(*Rules, s)
}

// String :
func (Rules *Rules) String() string {
	if len(*Rules) == 0 {
		return ""
	}
	return strings.Join(*Rules, "\n") + "\n"
}

// DockerFirewallRulesByChain :
type DockerFirewallRulesByChain map[string]*Rules

// DockerFirewallRulesByTable :
type DockerFirewallRulesByTable map[string]DockerFirewallRulesByChain

// DockerNetwork :
type DockerNetwork struct {
	*types.NetworkResource
	InterfaceName  string
	IsIPv4NAT      bool
	IPv4NATSubnets []string
}

// DockerNetworks :
type DockerNetworks []DockerNetwork

// DockerNetworkMap :
type DockerNetworkMap map[string]*DockerNetwork

// DockerFirewall :
type DockerFirewall struct {
	DockerClient

	Execute         bool
	Update          bool
	IPTablesRestore bool
	IPTablesCommand string
	Flush           bool

	AvailableTables   []string
	AvailableSections []string

	chainPrerouting  string
	chainPostrouting string
	chainOutput      string
	chainForward     string

	ChainDockerSNAT             string
	ChainDockerDNAT             string
	ChainDockerForward          string
	ChainDockerForwardIsolation string

	Rules DockerFirewallRulesByTable

	natTableSelected    bool
	filterTableSelected bool

	Containers []types.Container

	Networks     DockerNetworks
	NetworksByID DockerNetworkMap
}

// Init :
func (dockerFirewall *DockerFirewall) Init() {
	if len(dockerFirewall.IPTablesCommand) == 0 {
		dockerFirewall.IPTablesCommand = "iptables"
	}
	dockerFirewall.Rules = make(DockerFirewallRulesByTable)

	dockerFirewall.AvailableTables = []string{"nat", "filter"}
	dockerFirewall.AvailableSections = []string{"init", "docker", "root", "end"}

	dockerFirewall.chainForward = "FORWARD"
	dockerFirewall.chainOutput = "OUTPUT"
	dockerFirewall.chainPrerouting = "PREROUTING"
	dockerFirewall.chainPostrouting = "POSTROUTING"

	if len(dockerFirewall.ChainDockerSNAT) == 0 {
		dockerFirewall.ChainDockerSNAT = "DOCKER_SNAT"
	}

	if len(dockerFirewall.ChainDockerDNAT) == 0 {
		dockerFirewall.ChainDockerDNAT = "DOCKER_DNAT"
	}

	if len(dockerFirewall.ChainDockerForward) == 0 {
		dockerFirewall.ChainDockerForward = "DOCKER_FORWARD"
	}

	if len(dockerFirewall.ChainDockerForwardIsolation) == 0 {
		dockerFirewall.ChainDockerForwardIsolation = "DOCKER_ISOLATION"
	}

	dockerFirewall.Reset()

}

// Reset :
func (dockerFirewall *DockerFirewall) Reset() {
	for _, table := range dockerFirewall.AvailableTables {
		dockerFirewall.Rules[table] = make(DockerFirewallRulesByChain)
		dockerFirewall.Rules[table]["init"] = &Rules{}
		dockerFirewall.Rules[table][dockerFirewall.chainForward] = &Rules{}
		dockerFirewall.Rules[table][dockerFirewall.chainOutput] = &Rules{}
		dockerFirewall.Rules[table][dockerFirewall.chainPrerouting] = &Rules{}
		dockerFirewall.Rules[table][dockerFirewall.chainPostrouting] = &Rules{}
		dockerFirewall.Rules[table]["end"] = &Rules{}
	}

	dockerFirewall.Rules["nat"][dockerFirewall.ChainDockerSNAT] = &Rules{}
	dockerFirewall.Rules["nat"][dockerFirewall.ChainDockerDNAT] = &Rules{}
	dockerFirewall.Rules["filter"][dockerFirewall.ChainDockerForward] = &Rules{}
	dockerFirewall.Rules["filter"][dockerFirewall.ChainDockerForwardIsolation] = &Rules{}
}

// CollectData :
func (dockerFirewall *DockerFirewall) CollectData() error {
	dockerFirewall.Containers = nil
	dockerFirewall.Networks = nil
	dockerFirewall.NetworksByID = make(map[string]*DockerNetwork)

	if _networks, err := dockerFirewall.dockerClient.NetworkList(dockerFirewall.ctx, types.NetworkListOptions{}); err == nil {
		for networkIndex := range _networks {
			network := DockerNetwork{
				NetworkResource: &_networks[networkIndex],
			}
			if network.Driver == "bridge" {
				if _, ok := network.Options["com.docker.network.bridge.name"]; !ok {
					network.Options["com.docker.network.bridge.name"] = fmt.Sprintf("br-%s", network.ID[:12])
				}
				if _interfaceName, ok := network.Options["com.docker.network.bridge.name"]; ok {
					network.InterfaceName = _interfaceName
				}

				if network.IPAM.Driver == "default" {
					network.IsIPv4NAT = true
					for _, networkConfig := range network.IPAM.Config {
						if ipAddr, _, err := net.ParseCIDR(networkConfig.Subnet); err == nil {
							if ipAddr.To4() != nil {
								network.IPv4NATSubnets = append(network.IPv4NATSubnets, networkConfig.Subnet)
							}
						}
					}
				}

			}

			dockerFirewall.Networks = append(dockerFirewall.Networks, network)
			dockerFirewall.NetworksByID[network.ID] = &network

		}

		// sort so the result is consistent if there are no actual changes
		sort.SliceStable(dockerFirewall.Networks, func(a, b int) bool {
			return dockerFirewall.Networks[a].ID < dockerFirewall.Networks[b].ID
		})

		if _containers, err := dockerFirewall.dockerClient.ContainerList(dockerFirewall.ctx, types.ContainerListOptions{}); err == nil {
			dockerFirewall.Containers = _containers

			sort.SliceStable(dockerFirewall.Containers, func(a, b int) bool {
				return dockerFirewall.Containers[a].ID < dockerFirewall.Containers[b].ID
			})

			for _, container := range dockerFirewall.Containers {
				sort.SliceStable(container.Ports, func(a, b int) bool {
					if container.Ports[a].PublicPort == container.Ports[b].PublicPort {
						return container.Ports[a].Type < container.Ports[b].Type
					}
					return container.Ports[a].PublicPort < container.Ports[b].PublicPort
				})
			}

		} else {
			return err
		}
	} else {
		return err
	}

	return nil
}

func (dockerFirewall *DockerFirewall) iptablesCommand(table string) string {
	if dockerFirewall.IPTablesRestore {
		return ""
	}
	return fmt.Sprintf("%s -t %s ", dockerFirewall.IPTablesCommand, table)
}

func (dockerFirewall *DockerFirewall) appendLine(table string, chain string, line string) {
	if tableRules, ok := dockerFirewall.Rules[table]; ok {
		if rules, ok := tableRules[chain]; ok {
			rules.Append(line)
		}
	}
}

func (dockerFirewall *DockerFirewall) createChain(table string, chain string) {
	if tableRules, ok := dockerFirewall.Rules[table]; ok {
		if rules, ok := tableRules["init"]; ok {
			if !dockerFirewall.Update {
				if dockerFirewall.IPTablesRestore {
					rules.Append(fmt.Sprintf(":%s - [0:0]", chain))
				}
			}
			if !dockerFirewall.IPTablesRestore {
				rules.Append(
					fmt.Sprintf(
						"%s-N %s 2>/dev/null || true",
						dockerFirewall.iptablesCommand(table),
						chain,
					),
				)
				rules.Append(
					fmt.Sprintf(
						"%s-F %s",
						dockerFirewall.iptablesCommand(table),
						chain,
					),
				)
			}
		}
	}
}

func (dockerFirewall *DockerFirewall) removeChain(table string, chain string) {
	if tableRules, ok := dockerFirewall.Rules[table]; ok {
		if rules, ok := tableRules["end"]; ok {
			if !dockerFirewall.IPTablesRestore {
				rules.Append(
					fmt.Sprintf(
						"%s-F %s 2>/dev/null || true",
						dockerFirewall.iptablesCommand(table),
						chain,
					),
				)
				if !dockerFirewall.Update {
					rules.Append(
						fmt.Sprintf(
							"%s-X %s 2>/dev/null || true",
							dockerFirewall.iptablesCommand(table),
							chain,
						),
					)
				}
			}
		}
	}
}

// RuleOptions :
type RuleOptions struct {
	test      bool
	action    string
	noComment bool
}

func (dockerFirewall *DockerFirewall) appendRule(table string, chain string, rule string, options RuleOptions) {
	if tableRules, ok := dockerFirewall.Rules[table]; ok {
		if rules, ok := tableRules[chain]; ok {
			action := options.action
			if len(action) == 0 {
				action = "-A"
			}

			comment := " -m comment --comment '[DOCKER_FIREWALL]'"
			if options.noComment {
				comment = ""
			}
			command := " " + chain + " " + rule + comment
			if options.test {
				command = "if ( ! " + dockerFirewall.iptablesCommand(table) + "-C" + command + " 2>/dev/null ); then " + dockerFirewall.iptablesCommand(table) + action + command + "; fi"
			} else {
				command = dockerFirewall.iptablesCommand(table) + action + command
			}
			if action == "-D" {
				command += " 2>/dev/null || true"
			}
			rules.Append(command)
		}
	}
}
