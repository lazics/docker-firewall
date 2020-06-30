# Docker Firewall Manager

This utility can manage the firewall rules needed by Docker, _without_ the need for restarting Docker. It connects using the Docker API, collects the networks, running containers and their IP addresses, and generates iptables shell commands.

The ideal solution is to have it run automatically via netfilter-persistent (to create the root rules when the firewall starts) and as a service where docker-firewall is running in monitor mode (to update the rules when a container starts/stops). There are short-term plans to include `ufw` support as well.

It can also be invoked directly, with several output modes, if you want to integrate it in a custom solution.

## Build

Simply run `go build` . Requires Golang with modules support.


## Installation

This part assumes `Ubuntu 18.04` or higher.

First, we need to disable the iptables handling in Docker itself.

- stop Docker; make sure the old iptables rules are gone (this utility generates the chains a bit differently and with different chain names)

```bash
sudo systemctl stop docker.service
```

- add `"iptables": "false"` to `/etc/docker/daemon.json`. The file is in JSON format, if it doesn't exist, the complete file to be created would look like this:

```json
{
  "iptables": false
}
```

- start Docker

```bash
sudo systemctl start docker.service
```

Run it without arguments to test it; it will run in dry-run mode, without actually applying any rules, it just print them.

```bash
sudo ./docker-firewall
```

Install in the usual location:

```bash
sudo install -m 0755 docker-firewall /usr/sbin/docker-firewall
```

### netfilter-persistent

There is a plugin for *netfilter-persistent*, it should be copied/symlinked to the plugin directory, twice (needed to ensure the proper order of operations). 

For example on Ubuntu:

```bash
sudo cp "./netfilter-persistent/netfilter-persistent--docker-firewall" "/usr/share/netfilter-persistent/plugins.d/10-docker-firewall"
sudo cp "./netfilter-persistent/netfilter-persistent--docker-firewall" "/usr/share/netfilter-persistent/plugins.d/90-docker-firewall"
```

You can now restart netfilter-persistent, to apply the docker rules together with your own rules.

When using `netfilter-persistent save`, the dynamic rules will be removed before the rest of the rules are saved, and recreated afterwards, so you can use this command safely.

If you want to improve the position of the root rules (in the FORWARD, OUTPUT, PREROUTING, POSTROUTING chains), you can recreate the rules where you would like to have them, with the condition that the rules must be exactly the same.




To register as a service, there are two methods:

### Systemd

```bash
sudo cp "systemd/docker-firewall.service" "/lib/systemd/system/docker-firewall.service"
sudo systemctl daemon-reload
sudo systemctl enable docker-firewall.service
sudo systemctl start docker-firewall.service
```


### SysV script

This method is only used in case systemd is not available or simply not preferred.

```bash
sudo ln -s "/home/lazics/projects/infocus/alfred/docker/data/host-utils/docker-firewall/docker-firewall--rc.sh" "/etc/init.d/docker-firewall"
sudo update-rc.d docker-firewall defaults
sudo service docker-firewall start
```

## Command-line arguments

```
Usage: docker-firewall [-cefhmruv] [--inspect] [-i value] [--iptables value] [-o value] [-s value] [-t value] [parameters ...]
 -c, --change-only  Write/execute only if the output has changed
 -e, --execute      Execute the generated statements instead of just printing
                    them
 -f, --flush        Generate rules for removing the docker specific rules
                    instead
 -h, --help         Help
     --inspect      Dump the networks and containers, then exit
 -i, --invoke=value
                    Execute the specified executable
     --iptables=value
                    The iptables command (default: iptables)
 -m, --monitor      Monitor docker events continuously, update the rules when a
                    network event is received
 -o, --output=value
                    Write the generated statements to the specified file
 -r, --restore      Generate commands suitable for iptables-restore (remove the
                    'iptables' prefix, no test commands); Note: this is only a
                    partial output.
 -s, --section=value
                    The sections of the output to generate (init, docker, root,
                    end)
 -t, --table=value  The iptables table (filter, nat)
 -u, --update       Update the dynamic rules only (DOCKER_* chains), do not
                    create the initial rules in the FORWARD, OUTPUT, PREROUTING,
                    POSTROUTING chains
 -v, --verbose      Print debug messages
```

A few examples:

- Print the complete list of commands needed to create the root (the `JUMP` rules in `FORWARD`, `PREROUTING`, `OUTPUT`, `POSTROUTING` chains) and dynamic rules (the `DOCKER_*` chains):

```bash
sudo ./docker-firewall
```

See example output below. If you feel they are correct, you can actually apply the rules:

```bash
sudo ./docker-firewall --execute
```

To regenerate only the dynamic rules (will not touch the root rules, will create the chains for the dynamic rules if missing):
```
sudo ./docker-firewall --execute --update
```

To remove the dynamic and root rules respectively:
```
sudo ./docker-firewall --flush --execute
```

Can be combined with `--update` to remove only the dynamic rules (used by netfilter-persistent in save mde, to prevent saving the dynamic rules, while preserving the root rules)
```
sudo ./docker-firewall --flush --update --execute
```

To call an external executable after the rules have been generated (useful together with the file output feature):

```
sudo ./docker-firewall --output /tmp/rules.sh --change-only --invoke=/usr/local/bin/my-custom-firewall.sh
```

The called executable will receive the output file via the environment variable `DOCKER_FIREWALL_RULES`

### Monitor mode

In monitor mode the utility is watching continuously for network events from Docker, and triggers an update when such event occurs, to keep the rules up-to-date. This is used by the service mode (see below).

To preview the monitor functionality:
```
sudo ./docker-firewall --monitor
```

And of course, by adding `--execute` it will also apply the rules.

To display some debugging messages, `--verbose` can be used.


To write the output to a file instead:

```
sudo ./docker-firewall --output /tmp/rules.sh
```

To write the output to a file, but only if the rules changed:

```
sudo ./docker-firewall --output /tmp/rules.sh --change-only
```

The rest of the parameters are more irrelevant/debug/useless features, but if you see and use for them, have at it.

## TO-DO

- `ufw` integration
- further improve this page (admittedly there are holes in the explanations, but then again, there are many infrastructure possibilities with docker).
- packaging (.deb)
- binaries under *Releases*
- more testing


## Disclaimer

So far, this utility was only tested in a few specific environments, relying on the bridge and host network types. Other network types have not been tested yet. Basically it should be considered a beta version.

Also, since this is a side-project of an otherwise full-time job, bug reports will not be handled fast, although PRs are welcome. The best thing you can actually do is to complain to the folks at Docker about this crucial missing feature :D .



## Example of generated rules

```
## [DOCKER_FIREWALL] Table: nat Section: init
iptables -t nat -N DOCKER_DNAT 2>/dev/null || true
iptables -t nat -F DOCKER_DNAT
iptables -t nat -N DOCKER_SNAT 2>/dev/null || true
iptables -t nat -F DOCKER_SNAT

## [DOCKER_FIREWALL] Table: nat Section: docker
iptables -t nat -A DOCKER_DNAT -i br-0b3db5befd49 -j RETURN -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_DNAT -i docker0 -j RETURN -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_DNAT ! -i br-0b3db5befd49 -p tcp -m tcp --dport 80 -j DNAT --to-destination 10.0.250.2:80 -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_DNAT ! -i br-0b3db5befd49 -p tcp -m tcp --dport 443 -j DNAT --to-destination 10.0.250.2:443 -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_DNAT ! -i br-0b3db5befd49 -p tcp -m tcp --dport 20515 -j DNAT --to-destination 10.0.250.2:20514 -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_SNAT -s 10.0.250.0/24 ! -o br-0b3db5befd49 -j MASQUERADE -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_SNAT -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_SNAT -s 10.0.250.2 -d 10.0.250.2 -p tcp -m tcp --dport 80 -j MASQUERADE -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_SNAT -s 10.0.250.2 -d 10.0.250.2 -p tcp -m tcp --dport 443 -j MASQUERADE -m comment --comment '[DOCKER_FIREWALL]'
iptables -t nat -A DOCKER_SNAT -s 10.0.250.2 -d 10.0.250.2 -p tcp -m tcp --dport 20515 -j MASQUERADE -m comment --comment '[DOCKER_FIREWALL]'

## [DOCKER_FIREWALL] Table: nat Section: root
if ( ! iptables -t nat -C OUTPUT -j DOCKER_DNAT -m comment --comment '[DOCKER_FIREWALL]' 2>/dev/null ); then iptables -t nat -I OUTPUT -j DOCKER_DNAT -m comment --comment '[DOCKER_FIREWALL]'; fi
if ( ! iptables -t nat -C PREROUTING -j DOCKER_DNAT -m comment --comment '[DOCKER_FIREWALL]' 2>/dev/null ); then iptables -t nat -I PREROUTING -j DOCKER_DNAT -m comment --comment '[DOCKER_FIREWALL]'; fi
if ( ! iptables -t nat -C POSTROUTING -j DOCKER_SNAT -m comment --comment '[DOCKER_FIREWALL]' 2>/dev/null ); then iptables -t nat -I POSTROUTING -j DOCKER_SNAT -m comment --comment '[DOCKER_FIREWALL]'; fi

## [DOCKER_FIREWALL] Table: nat Section: end

## [DOCKER_FIREWALL] Table: filter Section: init
iptables -t filter -N DOCKER_FORWARD 2>/dev/null || true
iptables -t filter -F DOCKER_FORWARD
iptables -t filter -N DOCKER_ISOLATION 2>/dev/null || true
iptables -t filter -F DOCKER_ISOLATION

## [DOCKER_FIREWALL] Table: filter Section: docker
iptables -t filter -A DOCKER_FORWARD -i br-0b3db5befd49 ! -o br-0b3db5befd49 -j DOCKER_ISOLATION -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -o br-0b3db5befd49 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -i br-0b3db5befd49 -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -i docker0 ! -o docker0 -j DOCKER_ISOLATION -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -o docker0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -i docker0 -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -d 10.0.250.2 ! -i br-0b3db5befd49 -o br-0b3db5befd49 -p tcp -m tcp --dport 80 -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -d 10.0.250.2 ! -i br-0b3db5befd49 -o br-0b3db5befd49 -p tcp -m tcp --dport 443 -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_FORWARD -d 10.0.250.2 ! -i br-0b3db5befd49 -o br-0b3db5befd49 -p tcp -m tcp --dport 20515 -j ACCEPT -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_ISOLATION -o br-0b3db5befd49 -j DROP -m comment --comment '[DOCKER_FIREWALL]'
iptables -t filter -A DOCKER_ISOLATION -o docker0 -j DROP -m comment --comment '[DOCKER_FIREWALL]'

## [DOCKER_FIREWALL] Table: filter Section: root
if ( ! iptables -t filter -C FORWARD -j DOCKER_FORWARD -m comment --comment '[DOCKER_FIREWALL]' 2>/dev/null ); then iptables -t filter -I FORWARD -j DOCKER_FORWARD -m comment --comment '[DOCKER_FIREWALL]'; fi

## [DOCKER_FIREWALL] Table: filter Section: end
```

