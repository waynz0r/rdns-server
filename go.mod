module github.com/rancher/rdns-server

go 1.14

require (
	github.com/aws/aws-sdk-go v1.29.29
	github.com/caddyserver/caddy v1.0.5
	github.com/coredns/coredns v1.6.9
	github.com/go-sql-driver/mysql v1.5.0
	github.com/gorilla/context v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/miekg/dns v1.1.29
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/sirupsen/logrus v1.4.2
	github.com/urfave/cli v1.22.1
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200306183522-221f0cc107cb
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v0.17.4
	sigs.k8s.io/controller-runtime v0.5.0
)
