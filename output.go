package main

import "fmt"
import "strings"

// Output :
func (dockerFirewall *DockerFirewall) Output(table string, section string) string {
	result := fmt.Sprintf("## [DOCKER_FIREWALL] Table: %s Section: %s\n", table, section)

	storeRules := func(rules *Rules) {
		if rules != nil && len(*rules) > 0 {
			result += strings.Join(*rules, "\n") + "\n"
		}
	}

	keys := []string{}
	if section == "init" {
		keys = append(keys, "init")
	}
	if section == "docker" {
		keys = append(keys,
			dockerFirewall.ChainDockerDNAT,
			dockerFirewall.ChainDockerSNAT,

			dockerFirewall.ChainDockerForward,
			dockerFirewall.ChainDockerForwardIsolation,
		)
	}
	if section == "root" {
		keys = append(keys,
			dockerFirewall.chainOutput,
			dockerFirewall.chainPrerouting,
			dockerFirewall.chainPostrouting,

			dockerFirewall.chainForward,
		)
	}
	if section == "end" {
		keys = append(keys, "end")
	}

	if tableRules, ok := dockerFirewall.Rules[table]; ok {
		for _, k := range keys {
			if rules, ok := tableRules[k]; ok {
				storeRules(rules)
			}
		}
	}

	return result
}
