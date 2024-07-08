/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	durosensurer "github.com/metal-stack/duros-controller/cmd/duros-seed/internal"
	"github.com/metal-stack/duros-go"
)

func main() {
	var (
		logLevel    string
		metricsAddr string
		adminToken  string
		adminKey    string
		endpoints   string
		// apiEndpoint is the duros-grpc-proxy with client cert validation
		apiEndpoint string
		apiCA       string
		apiKey      string
		apiCert     string
	)
	flag.StringVar(&logLevel, "log-level", "", "The log level of the controller.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&adminToken, "admin-token", "/duros/admin-token", "The admin token file for the duros api.")
	flag.StringVar(&adminKey, "admin-key", "/duros/admin-key", "The admin key file for the duros api.")
	flag.StringVar(&endpoints, "endpoints", "", "The endpoints, in the form host:port,host:port of the duros api.")

	flag.StringVar(&apiEndpoint, "api-endpoint", "", "The api endpoint, in the form host:port of the duros api")
	flag.StringVar(&apiCA, "api-ca", "", "The api endpoint ca")
	flag.StringVar(&apiCert, "api-cert", "", "The api endpoint cert")
	flag.StringVar(&apiKey, "api-key", "", "The api endpoint key")

	flag.Parse()

	level := slog.LevelInfo
	if len(logLevel) > 0 {
		var lvlvar slog.LevelVar
		err := lvlvar.UnmarshalText([]byte(logLevel))
		if err != nil {
			slog.Error("can't initialize logger", "err", err)
			os.Exit(1)
		}
		level = lvlvar.Level()
	}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	l := slog.New(jsonHandler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// connect to duros

	if err := validateEndpoints(apiEndpoint); err != nil {
		l.Error("unable to parse api-endpoint", "err", err)
		os.Exit(1)
	}
	at, err := os.ReadFile(adminToken)
	if err != nil {
		l.Error("unable to read admin-token from file", "err", err)
		os.Exit(1)
	}
	_, err = os.ReadFile(adminKey)
	if err != nil {
		l.Error("unable to read admin-key from file", "err", err)
		os.Exit(1)
	}
	durosConfig := duros.DialConfig{
		Token:     string(at),
		Endpoint:  apiEndpoint,
		Scheme:    duros.GRPCS,
		Log:       l,
		UserAgent: "duros-controller",
	}

	if apiCA != "" && apiCert != "" && apiKey != "" {
		l.Info("connecting to api with client cert", "api-endpoint", apiEndpoint)
		ac, err := os.ReadFile(apiCA)
		if err != nil {
			l.Error("unable to read api-ca from file", "err", err)
			os.Exit(1)
		}
		ace, err := os.ReadFile(apiCert)
		if err != nil {
			l.Error("unable to read api-cert from file", "err", err)
			os.Exit(1)
		}
		ak, err := os.ReadFile(apiKey)
		if err != nil {
			l.Error("unable to read api-key from file", "err", err)
			os.Exit(1)
		}
		serverName, _, err := net.SplitHostPort(apiEndpoint)
		if err != nil {
			l.Error("unable to parse api-endpoint", "err", err)
			os.Exit(1)
		}

		creds := &duros.ByteCredentials{
			CA:         ac,
			Cert:       ace,
			Key:        ak,
			ServerName: serverName,
		}
		durosConfig.ByteCredentials = creds
	}
	durosClient, err := duros.Dial(ctx, durosConfig)
	if err != nil {
		l.Error("cannot connect to duros api", "err", err)
		os.Exit(1)
	}

	var policies []durosensurer.QoSPolicyDef
	ensurer := durosensurer.NewEnsurer(l, durosClient)
	if err := ensurer.EnsurePolicies(ctx, policies); err != nil {
		l.Error("failed to ensure duros resources", "err", err)
		os.Exit(1)
	}
}

func validateEndpoints(endpoints string) error {
	for _, endpoint := range strings.Split(endpoints, ",") {
		host, port, err := net.SplitHostPort(strings.TrimSpace(endpoint))
		if err != nil {
			return err
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("invalid empty host")
		}
		if _, err = strconv.ParseUint(port, 10, 16); err != nil {
			return err
		}
	}
	return nil
}
