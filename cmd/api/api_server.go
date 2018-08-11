package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/fagongzi/gateway/pkg/pb/rpcpb"
	"github.com/fagongzi/gateway/pkg/service"
	"github.com/fagongzi/gateway/pkg/store"
	"github.com/fagongzi/gateway/pkg/util"
	"github.com/fagongzi/grpcx"
	"github.com/fagongzi/log"
	"google.golang.org/grpc"
	"strconv"
)

//var (
//	addr           = flag.String("addr", "127.0.0.1:9092", "Addr: client grpc entrypoint")
//	addrHTTP       = flag.String("addr-http", "127.0.0.1:9093", "Addr: client http restful entrypoint")
//	addrStore      = flag.String("addr-store", "etcd://127.0.0.1:2379", "Addr: store address")
//	namespace      = flag.String("namespace", "dev", "The namespace to isolation the environment.")
//	discovery      = flag.Bool("discovery", false, "Publish apiserver service via discovery.")
//	servicePrefix  = flag.String("service-prefix", "/services", "The prefix for service name.")
//	publishLease   = flag.Int64("publish-lease", 10, "Publish service lease seconds")
//	publishTimeout = flag.Int("publish-timeout", 30, "Publish service timeout seconds")
//	version        = flag.Bool("version", false, "Show version info")
//)
var (
	addr           = flag.String("addr", os.Getenv("ADDR_ENV"), "Addr: client grpc entrypoint")
	addrHTTP       = flag.String("addr-http", os.Getenv("ADDR_HTTP_ENV"), "Addr: client http restful entrypoint")
	addrStore      = flag.String("addr-store", os.Getenv("ADDR_STORE_ENV"), "Addr: store address")
	namespace      = flag.String("namespace", os.Getenv("NAMESPACE_ENV"), "The namespace to isolation the environment.")
	discovery_env,_ = strconv.ParseBool(os.Getenv("DISCOVERY_ENV"))
	discovery      = flag.Bool("discovery", discovery_env, "Publish apiserver service via discovery.")
	servicePrefix  = flag.String("service-prefix", os.Getenv("SERVICE_PREFIX_ENV"), "The prefix for service name.")
	publishLease_env,_ = strconv.ParseInt(os.Getenv("PUBLISH_LEASE_ENV"),10,64)
	publishLease   = flag.Int64("publish-lease", publishLease_env, "Publish service lease seconds")
	publisTimeout_env,_ = strconv.Atoi(os.Getenv("PUBLISH_TIMEOUT_ENV"))
	publishTimeout = flag.Int("publish-timeout", publisTimeout_env, "Publish service timeout seconds")
	version_env,_ = strconv.ParseBool(os.Getenv("VERSION_ENV"))
	version        = flag.Bool("version", version_env, "Show version info")
)

func main() {
	flag.Parse()

	if *version && util.PrintVersion() {
		os.Exit(0)
	}

	log.InitLog()
	runtime.GOMAXPROCS(runtime.NumCPU())

	log.Infof("addr: %s", *addr)
	log.Infof("addr-store: %s", *addrStore)
	log.Infof("namespace: %s", *namespace)
	log.Infof("discovery: %v", *discovery)
	log.Infof("service-prefix: %s", *servicePrefix)
	log.Infof("publish-lease: %d", *publishLease)
	log.Infof("publish-timeout: %d", *publishTimeout)

	db, err := store.GetStoreFrom(*addrStore, fmt.Sprintf("/%s", *namespace))
	if err != nil {
		log.Fatalf("init store failed for %s, errors:\n%+v",
			*addrStore,
			err)
	}

	service.Init(db)

	var opts []grpcx.ServerOption
	if *discovery {
		opts = append(opts, grpcx.WithEtcdPublisher(db.Raw().(*clientv3.Client), *servicePrefix, *publishLease, time.Second*time.Duration(*publishTimeout)))
	}

	if *addrHTTP != "" {
		opts = append(opts, grpcx.WithHTTPServer(*addrHTTP, service.InitHTTPRouter))
	}

	s := grpcx.NewGRPCServer(*addr, func(svr *grpc.Server) []grpcx.Service {
		var services []grpcx.Service
		rpcpb.RegisterMetaServiceServer(svr, service.MetaService)
		services = append(services, grpcx.NewService(rpcpb.ServiceMeta, nil))
		return services
	}, opts...)

	log.Infof("api server listen at %s", *addr)
	go s.Start()

	waitStop(s)
}

func waitStop(s *grpcx.GRPCServer) {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	sig := <-sc
	s.GracefulStop()
	log.Infof("exit: signal=<%d>.", sig)
	switch sig {
	case syscall.SIGTERM:
		log.Infof("exit: bye :-).")
		os.Exit(0)
	default:
		log.Infof("exit: bye :-(.")
		os.Exit(1)
	}
}
