package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/endpoint"
	"github.com/usnistgov/ndn-dpdk/ndn/l3"
	"github.com/usnistgov/ndn-dpdk/ndn/sockettransport"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

const (
	fchUriBase   = "https://ndn-fch.named-data.net"
	fchCount     = 4
	fchProbeName = "/localhop/nfd/rib/list"
)

func connectToTestbed(ctx context.Context) error {
	fchLogger := logger.Named("FCH")

	resp, e := http.Get(fmt.Sprintf("%s/?k=%d", fchUriBase, fchCount))
	if e != nil {
		return fmt.Errorf("NDN-FCH HTTP %w", e)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NDN-FCH HTTP %d", resp.StatusCode)
	}
	body, e := io.ReadAll(resp.Body)
	if e != nil {
		return fmt.Errorf("NDN-FCH HTTP %w", e)
	}
	routers := bytes.Split(body, []byte(","))
	fchLogger.Info("NDN-FCH response", zap.ByteStrings("routers", routers))
	rand.Shuffle(len(routers), reflect.Swapper(routers))

	connectErrors := []error{}
	for _, routerB := range routers {
		router := net.JoinHostPort(string(routerB), "6363")
		if e := connectToRouter(ctx, router, true); e == nil {
			fchLogger.Info("connected", zap.String("router", router))
			return nil
		}
		connectErrors = append(connectErrors, fmt.Errorf("%s %w", router, e))
	}
	return multierr.Combine(connectErrors...)
}

func connectToRouter(ctx context.Context, router string, sendProbeInterest bool) (e error) {
	network := "udp"
	if strings.HasPrefix(router, "/") {
		network = "unix"
	}
	tr, e := sockettransport.Dial(network, "", router)
	if e != nil {
		return e
	}

	l3face, e := l3.NewFace(tr)
	if e != nil {
		return e
	}

	face, e := l3.GetDefaultForwarder().AddFace(l3face)
	if e != nil {
		return e
	}
	face.AddRoute(ndn.Name{})
	if !sendProbeInterest {
		return nil
	}

	timeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	interest := ndn.MakeInterest(fchProbeName, ndn.CanBePrefixFlag, ndn.MustBeFreshFlag)
	if _, e := endpoint.Consume(timeout, interest, endpoint.ConsumerOptions{}); e != nil {
		face.Close()
		return fmt.Errorf("test Interest %w", e)
	}

	return nil
}
