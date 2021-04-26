package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/segmented"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger = func() *zap.Logger {
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		os.Stderr,
		zap.DebugLevel,
	)
	return zap.New(core)
}()

var (
	listenAddr      string
	upstreamScheme  string
	upstreamHost    string
	upstreamPrefix  ndn.Name
	ndnFetchOptions segmented.FetchOptions
	ndnFetchTimeout = 10 * time.Minute
)

var app = &cli.App{
	Name: "Docker-registry-NDN-client",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "listen",
			Usage:       "HTTP server address",
			Destination: &listenAddr,
			Value:       "127.0.0.1:5000",
		},
		&cli.StringFlag{
			Name:  "upstream",
			Usage: "upstream registry HTTP",
			Value: "https://docker.yoursunny.dev",
		},
		&cli.StringFlag{
			Name:  "name",
			Usage: "upstream registry NDN prefix",
			Value: "/yoursunny/docker",
		},
		&cli.StringFlag{
			Name:  "router",
			Usage: "use specific NDN router (UDP host:port or unix socket path)",
		},
		&cli.IntFlag{
			Name:        "retx",
			Usage:       "Interest retransmission limit",
			Destination: &ndnFetchOptions.RetxLimit,
			Value:       10,
		},
		&cli.IntFlag{
			Name:        "max-cwnd",
			Usage:       "maximum congestion window",
			Destination: &ndnFetchOptions.MaxCwnd,
			Value:       32,
		},
		&cli.DurationFlag{
			Name:        "timeout",
			Usage:       "blob retrieval timeout",
			Destination: &ndnFetchTimeout,
			Value:       10 * time.Minute,
		},
	},
	Before: func(c *cli.Context) (e error) {
		upstreamUri, e := url.Parse(c.String("upstream"))
		if e != nil {
			return cli.Exit(fmt.Errorf("bad upstream %w", e), 1)
		}
		if path := strings.TrimPrefix(upstreamUri.Path, "/"); path != "" {
			logger.Warn("ignored path on upstream URI", zap.Stringer("uri", upstreamUri))
		}
		upstreamScheme, upstreamHost = upstreamUri.Scheme, upstreamUri.Host

		upstreamPrefix = ndn.ParseName(c.String("name"))

		if router := c.String("router"); router != "" {
			e = connectToRouter(c.Context, router, false)
		} else {
			e = connectToTestbed(c.Context)
		}
		if e != nil {
			return cli.Exit(e, 1)
		}

		return nil
	},
	Action: func(c *cli.Context) error {
		http.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("User-Agent: *\nDisallow: /\n"))
		})
		http.HandleFunc("/v2/", handleV2)
		return cli.Exit(http.ListenAndServe(listenAddr, nil), 1)
	},
}

func main() {
	rand.Seed(time.Now().UnixNano())
	app.Run(os.Args)
}
