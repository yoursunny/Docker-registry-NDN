package main

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/endpoint"
	"github.com/usnistgov/ndn-dpdk/ndn/l3"
	"github.com/usnistgov/ndn-dpdk/ndn/sockettransport"
	"github.com/yoursunny/Docker-registry-NDN/client/fch"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

var (
	probeInterestName = "/localhop/nfd/rib/list"
)

func connectToNetwork(ctx context.Context) error {
	fchLogger := logger.Named("FCH")
	res, e := fch.Query(ctx, fch.Request{Count: 4})
	if e != nil {
		fchLogger.Warn("query error", zap.Error(e))
		return fmt.Errorf("NDN-FCH %w", e)
	}

	fchLogger.Info("response", zap.Any("res", res))
	rand.Shuffle(len(res.Routers), reflect.Swapper(res.Routers))

	connectErrors := []error{}
	for _, router := range res.Routers {
		if e = connectToRouter(ctx, router.Connect); e == nil {
			fchLogger.Info("connected", zap.String("router", router.Connect))
			return nil
		}
		connectErrors = append(connectErrors, fmt.Errorf("%s %w", router, e))
	}
	return multierr.Combine(connectErrors...)
}

func connectToRouter(ctx context.Context, router string) (e error) {
	network := "udp"
	if strings.HasPrefix(router, "/") {
		network = "unix"
	}
	tr, e := sockettransport.Dial(network, "", router)
	if e != nil {
		return e
	}

	l3face, e := l3.NewFace(tr, l3.FaceConfig{})
	if e != nil {
		return e
	}

	face, e := l3.GetDefaultForwarder().AddFace(l3face)
	if e != nil {
		return e
	}
	face.AddRoute(ndn.Name{})
	if probeInterestName == "" {
		return nil
	}

	timeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	interest := ndn.MakeInterest(probeInterestName, ndn.CanBePrefixFlag, ndn.MustBeFreshFlag)
	if _, e := endpoint.Consume(timeout, interest, endpoint.ConsumerOptions{}); e != nil {
		face.Close()
		return fmt.Errorf("test Interest %w", e)
	}

	return nil
}
