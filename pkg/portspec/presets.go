package portspec

import (
	"maps"
	"slices"
)

// commonPorts is a curated list of ports frequently used by developers and
// common infrastructure. It is intentionally short and hand-maintained.
var commonPorts = []uint16{
	20, 21, // ftp
	22,    // ssh
	23,    // telnet
	25,    // smtp
	53,    // dns
	80,    // http
	110,   // pop3
	123,   // ntp
	143,   // imap
	389,   // ldap
	443,   // https
	445,   // smb
	465,   // smtps
	587,   // smtp submission
	636,   // ldaps
	993,   // imaps
	995,   // pop3s
	1433,  // mssql
	1521,  // oracle
	2049,  // nfs
	2375,  // docker
	2376,  // docker tls
	3000,  // dev servers (node, rails, grafana)
	3306,  // mysql
	3389,  // rdp
	4200,  // angular dev server
	5000,  // flask / dev servers
	5173,  // vite
	5432,  // postgresql
	5672,  // amqp
	5900,  // vnc
	6379,  // redis
	6443,  // kubernetes api
	8000,  // dev servers
	8080,  // http alt
	8443,  // https alt
	8888,  // jupyter
	9000,  // misc dev / php-fpm
	9090,  // prometheus
	9200,  // elasticsearch
	9092,  // kafka
	11211, // memcached
	15672, // rabbitmq management
	27017, // mongodb
}

// presets maps preset names (lower case) to their ranges.
var presets = map[string][]Range{
	"common":     singles(commonPorts),
	"well-known": {{Start: 1, End: 1023}},
	"registered": {{Start: 1024, End: 49151}},
	"dynamic":    {{Start: 49152, End: 65535}},
	"all":        {{Start: MinPort, End: MaxPort}},
}

func singles(ports []uint16) []Range {
	rs := make([]Range, len(ports))
	for i, p := range ports {
		rs[i] = Range{Start: p, End: p}
	}
	return rs
}

// Presets returns the names of the built-in presets in sorted order.
func Presets() []string {
	return slices.Sorted(maps.Keys(presets))
}
