package main

import "fmt"
import "log"
import "os"
import "os/exec"
import "crypto/sha256"
import "io"
import "io/ioutil"
import "bytes"
import "time"

import "github.com/pborman/getopt/v2"

import "github.com/davecgh/go-spew/spew"

var verbose bool

// EventMonitor :
type EventMonitor struct {
	DockerClient

	monitorChannel chan bool
	debounceTimer  *time.Timer
}

// Init :
func (eventMonitor *EventMonitor) Init() {
	eventMonitor.monitorChannel = make(chan bool)
}

// Run :
func (eventMonitor *EventMonitor) Run() {

	for {
		if err := eventMonitor.Connect(); err != nil {
			log.Println(err)
		} else {
			log.Println("Monitoring events...")
			eventMonitor.MonitorNetworkEvents()

		MonitorLoop:
			for {
				select {
				case err := <-eventMonitor.eventErrorChannel:
					log.Println(err)
					break MonitorLoop
				case message := <-eventMonitor.eventMessageChannel:
					if verbose {
						log.Println(spew.Sdump(message))
					}
					if eventMonitor.debounceTimer != nil {
						eventMonitor.debounceTimer.Stop()
					}
					eventMonitor.debounceTimer = time.AfterFunc(5*time.Second, eventMonitor.notify)
				}
			}
		}

		eventMonitor.Close()

		time.Sleep(1 * time.Second)

		log.Println("Retrying...")

	}
}

func (eventMonitor *EventMonitor) notify() {
	log.Println("notify")
	eventMonitor.monitorChannel <- true
}

func main() {

	dockerFirewall := DockerFirewall{}

	help := false
	inspect := false
	outputFileName := ""
	changeOnly := false
	execute := false
	invoke := ""
	tables := []string{}
	sections := []string{}
	monitor := false
	eventMonitor := EventMonitor{}

	getopt.FlagLong(&help, "help", 'h', "Help")

	getopt.FlagLong(&verbose, "verbose", 'v', "Print debug messages")
	getopt.FlagLong(&inspect, "inspect", 0, "Dump the networks and containers, then exit")
	getopt.FlagLong(&outputFileName, "output", 'o', "Write the generated statements to the specified file")
	getopt.FlagLong(&execute, "execute", 'e', "Execute the generated statements instead of just printing them")
	getopt.FlagLong(&invoke, "invoke", 'i', "Execute the specified executable")
	getopt.FlagLong(&monitor, "monitor", 'm', "Monitor docker events continuously, update the rules when a network event is received")
	getopt.FlagLong(&changeOnly, "change-only", 'c', "Write/execute only if the output has changed")
	getopt.FlagLong(&dockerFirewall.Update, "update", 'u', "Update the dynamic rules only (DOCKER_* chains), do not create the initial rules in the FORWARD, OUTPUT, PREROUTING, POSTROUTING chains")
	getopt.FlagLong(&dockerFirewall.Flush, "flush", 'f', "Generate rules for removing the docker specific rules instead")
	getopt.FlagLong(&dockerFirewall.IPTablesRestore, "restore", 'r', "Generate commands suitable for iptables-restore (remove the 'iptables' prefix, no test commands); Note: this is only a partial output.")
	getopt.FlagLong(&dockerFirewall.IPTablesCommand, "iptables", 0, "The iptables command (default: iptables)")

	getopt.FlagLong(&tables, "table", 't', "The iptables table (filter, nat)")
	getopt.FlagLong(&sections, "section", 's', "The sections of the output to generate (init, docker, root, end)")

	getopt.Parse()
	if help {
		getopt.Usage()
		os.Exit(0)
	}

	dockerFirewall.Init()

	if len(tables) == 0 {
		tables = dockerFirewall.AvailableTables
	}

	if len(sections) == 0 {
		sections = dockerFirewall.AvailableSections
	}

	eventMonitor.Init()

	if monitor {
		dockerFirewall.Update = true
		go eventMonitor.Run()
	}

	for {

		if monitor {
			switch <-eventMonitor.monitorChannel {
			case true:
				log.Println("Updating docker firewall rules...")
			}
		}

		dockerFirewall.Reset()

		if err := dockerFirewall.Connect(); err != nil {
			log.Println(err)
		} else {

			if err := dockerFirewall.CollectData(); err != nil {
				log.Println(err)
			} else {
				dockerFirewall.Close()

				changed := true

				if inspect && !monitor {

					fmt.Println("############ Networks ##############")
					for _, network := range dockerFirewall.Networks {
						fmt.Println("\n\n\n### Network ", network.ID, "\n")
						fmt.Println("network: ", spew.Sdump(network))
					}

					fmt.Println("\n\n\n############ Containers ##############")
					for _, container := range dockerFirewall.Containers {
						fmt.Println("\n\n\n### Container ", container.ID, "\n")
						fmt.Println("container: ", spew.Sdump(container))
					}

					os.Exit(0)
				}

				if err := dockerFirewall.Generate(); err != nil {
					panic(err)
				}

				result := ""
				for _, table := range tables {
					for _, section := range sections {
						result += dockerFirewall.Output(table, section) + "\n"
					}
				}

				if !monitor && len(outputFileName) > 0 {
					resultHash := sha256.Sum256([]byte(result))

					var outputFileMode os.FileMode
					outputFileMode = 0755
					if dockerFirewall.IPTablesRestore {
						outputFileMode = 0644
					}

					if changeOnly {
						if verbose {
							log.Println("Loading existing file...")
						}
						if f, err := os.Open(outputFileName); err == nil {
							defer f.Close()

							h := sha256.New()
							if _, err := io.Copy(h, f); err == nil {
								changed = !bytes.Equal(h.Sum(nil), resultHash[:])
								if verbose {
									if changed {
										log.Println("Rules have changed")
									} else {
										log.Println("No change...")
									}
								}
							} else {
								fmt.Println(err)
							}
						} else {
							if !os.IsNotExist(err) {
								fmt.Println(err)
							} else {
								if verbose {
									log.Println("Missing old file")
								}
							}
						}
					}

					if changed {
						if verbose {
							log.Println("Writing output to: ", outputFileName)
						}
						if err := ioutil.WriteFile(outputFileName, []byte(result), outputFileMode); err != nil {
							panic(err)
						}
					}
				}

				if changed {
					if len(outputFileName) == 0 && !execute && len(invoke) == 0 {
						fmt.Println(result)
					}

					if execute {
						if dockerFirewall.IPTablesRestore {
							panic("Can't execute iptables-restore file format")
						}

						if verbose {
							log.Println("Executing...")
						}

						cmd := exec.Command("/bin/bash", "-s")
						if stdin, err := cmd.StdinPipe(); err == nil {
							go func() {
								defer stdin.Close()
								io.WriteString(stdin, "set -e\n")
								if verbose {
									io.WriteString(stdin, "set -x\n")
								}
								io.WriteString(stdin, result)
							}()

							if output, err := cmd.CombinedOutput(); err != nil {
								fmt.Println(string(output))
								fmt.Println(err)
							} else {
								if verbose {
									fmt.Println(string(output))
									log.Println("Finished")
								}
							}

						} else {
							log.Println(err)
						}
					}

					if len(invoke) > 0 {
						if verbose {
							log.Println("Invoking", invoke)
						}
						cmd := exec.Command(invoke)
						if len(outputFileName) > 0 {
							cmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_FIREWALL_RULES=%s", outputFileName))
						} else {
							if stdin, err := cmd.StdinPipe(); err == nil {
								go func() {
									defer stdin.Close()
									io.WriteString(stdin, result)
								}()
							} else {
								log.Println(err)
							}
						}

						if output, err := cmd.CombinedOutput(); err != nil {
							fmt.Println(string(output))
							log.Println(err)
						} else {
							fmt.Println(string(output))
							if verbose {
								log.Println("Finished")
							}
						}

					}
				}
			}
		}

		if !monitor {
			break
		}
	}

}
