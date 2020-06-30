package main

import "fmt"

// Generate :
func (dockerFirewall *DockerFirewall) Generate() error {

	if !dockerFirewall.Update {
		rootRuleOptions := RuleOptions{test: true, action: "-I"}
		if dockerFirewall.IPTablesRestore {
			rootRuleOptions = RuleOptions{}
		}
		if dockerFirewall.Flush {
			rootRuleOptions.test = false
			rootRuleOptions.action = "-D"
		}

		dockerFirewall.appendRule(
			"nat", dockerFirewall.chainPrerouting,
			fmt.Sprintf("-j %s",
				dockerFirewall.ChainDockerDNAT,
			),
			rootRuleOptions,
		)

		dockerFirewall.appendRule(
			"nat", dockerFirewall.chainOutput,
			fmt.Sprintf("-j %s",
				dockerFirewall.ChainDockerDNAT,
			),
			rootRuleOptions,
		)

		dockerFirewall.appendRule(
			"nat", dockerFirewall.chainPostrouting,
			fmt.Sprintf("-j %s",
				dockerFirewall.ChainDockerSNAT,
			),
			rootRuleOptions,
		)

		dockerFirewall.appendRule(
			"filter", dockerFirewall.chainForward,
			fmt.Sprintf("-j %s",
				dockerFirewall.ChainDockerForward,
			),
			rootRuleOptions,
		)
	}

	if dockerFirewall.Flush {
		dockerFirewall.removeChain("nat", dockerFirewall.ChainDockerDNAT)
		dockerFirewall.removeChain("nat", dockerFirewall.ChainDockerSNAT)
		dockerFirewall.removeChain("filter", dockerFirewall.ChainDockerForward)
		dockerFirewall.removeChain("filter", dockerFirewall.ChainDockerForwardIsolation)
	} else {
		dockerFirewall.createChain("nat", dockerFirewall.ChainDockerDNAT)
		dockerFirewall.createChain("nat", dockerFirewall.ChainDockerSNAT)
		dockerFirewall.createChain("filter", dockerFirewall.ChainDockerForward)
		dockerFirewall.createChain("filter", dockerFirewall.ChainDockerForwardIsolation)

		for _, network := range dockerFirewall.Networks {
			if network.IsIPv4NAT {

				for _, subnet := range network.IPv4NATSubnets {
					dockerFirewall.appendRule(
						"nat", dockerFirewall.ChainDockerSNAT,
						fmt.Sprintf("-s %s ! -o %s -j MASQUERADE",
							subnet,
							network.InterfaceName,
						),
						RuleOptions{},
					)

				}
				dockerFirewall.appendRule(
					"nat", dockerFirewall.ChainDockerDNAT,
					fmt.Sprintf("-i %s -j RETURN",
						network.InterfaceName,
					),
					RuleOptions{},
				)

				dockerFirewall.appendRule(
					"filter", dockerFirewall.ChainDockerForwardIsolation,
					fmt.Sprintf("-o %s -j DROP",
						network.InterfaceName,
					),
					RuleOptions{},
				)

				dockerFirewall.appendRule(
					"filter", dockerFirewall.ChainDockerForward,
					fmt.Sprintf("-i %s ! -o %s -j %s",
						network.InterfaceName,
						network.InterfaceName,
						dockerFirewall.ChainDockerForwardIsolation,
					),
					RuleOptions{},
				)

				dockerFirewall.appendRule(
					"filter", dockerFirewall.ChainDockerForward,
					fmt.Sprintf("-o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
						network.InterfaceName,
					),
					RuleOptions{},
				)

				dockerFirewall.appendRule(
					"filter", dockerFirewall.ChainDockerForward,
					fmt.Sprintf("-i %s -j ACCEPT",
						network.InterfaceName,
					),
					RuleOptions{},
				)
			}
		}

		for _, container := range dockerFirewall.Containers {
			for _, containerNetwork := range container.NetworkSettings.Networks {
				if network, ok := dockerFirewall.NetworksByID[containerNetwork.NetworkID]; ok {
					if network.IsIPv4NAT {
						for _, port := range container.Ports {
							dockerFirewall.appendRule(
								"nat", dockerFirewall.ChainDockerDNAT,
								fmt.Sprintf("! -i %s -p %s -m %s --dport %d -j DNAT --to-destination %s:%d",
									network.InterfaceName,
									port.Type,
									port.Type,
									port.PublicPort,
									containerNetwork.IPAddress,
									port.PrivatePort,
								),
								RuleOptions{},
							)

							dockerFirewall.appendRule(
								"nat", dockerFirewall.ChainDockerSNAT,
								fmt.Sprintf("-s %s -d %s -p %s -m %s --dport %d -j MASQUERADE",
									containerNetwork.IPAddress,
									containerNetwork.IPAddress,
									port.Type,
									port.Type,
									port.PublicPort,
								),
								RuleOptions{},
							)

							dockerFirewall.appendRule(
								"filter", dockerFirewall.ChainDockerForward,
								fmt.Sprintf("-d %s ! -i %s -o %s -p %s -m %s --dport %d -j ACCEPT",
									containerNetwork.IPAddress,
									network.InterfaceName,
									network.InterfaceName,
									port.Type,
									port.Type,
									port.PublicPort,
								),
								RuleOptions{},
							)
						}
					}
				}
			}
		}
	}

	return nil
}
