package scan

// services maps well-known TCP ports to service names (IANA registry and
// common de-facto assignments). It is intentionally small: it labels results,
// it does not identify what is actually running.
var services = map[uint16]string{
	20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "domain",
	80: "http", 110: "pop3", 111: "rpcbind", 123: "ntp", 135: "msrpc", 139: "netbios-ssn",
	143: "imap", 161: "snmp", 389: "ldap", 443: "https", 445: "microsoft-ds",
	465: "smtps", 514: "syslog", 587: "submission", 631: "ipp", 636: "ldaps",
	873: "rsync", 993: "imaps", 995: "pop3s", 1080: "socks", 1433: "ms-sql-s",
	1521: "oracle", 1883: "mqtt", 2049: "nfs", 2375: "docker", 2376: "docker-tls",
	2379: "etcd-client", 2380: "etcd-server", 3000: "dev-http", 3306: "mysql",
	3389: "ms-wbt-server", 4222: "nats", 5000: "dev-http", 5173: "vite",
	5432: "postgresql", 5601: "kibana", 5672: "amqp", 5900: "vnc", 6379: "redis",
	6443: "kubernetes", 7000: "cassandra", 8000: "http-alt", 8080: "http-proxy",
	8443: "https-alt", 8500: "consul", 8888: "jupyter", 9000: "http-alt",
	9042: "cassandra-cql", 9090: "prometheus", 9092: "kafka", 9200: "elasticsearch",
	9300: "elasticsearch-transport", 10250: "kubelet", 11211: "memcached",
	15672: "rabbitmq-mgmt", 27017: "mongodb",
}

// Service returns the well-known name for a TCP port, or "".
func Service(port uint16) string { return services[port] }
