package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type IPerf3Summary struct {
	Start         float32 `json:"start"`
	End           float32 `json:"end"`
	Seconds       float32 `json:"seconds"`
	Bytes         int     `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Retransmits   float64 `json:"retransmits"`
}

type IPerf3Result struct {
	Error string `json:"error"`
	End   struct {
		SumSent     IPerf3Summary `json:"sum_sent"`
		SumReceived IPerf3Summary `json:"sum_received"`
	} `json:"end"`
}

func ExecIPerf3(server, port string) (*IPerf3Result, error) {
	stdout := bytes.NewBuffer(nil)
	cmd := exec.Command("iperf3", "--json", "-c", server, "-p", port, "--connect-timeout", "1000")
	cmd.Stdout = stdout
	result := &IPerf3Result{}
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	exitCode := cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		return nil, fmt.Errorf("exit code: %d", exitCode)
	}
	err = json.Unmarshal(stdout.Bytes(), result)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, errors.New(result.Error)
	}
	return result, err
}

func main() {
	server := flag.String("server", "127.0.0.1", "iperf3 server ip")
	port := flag.String("port", "5201", "iperf3 server port")
	interval := flag.String("interval", "5m", "iperf3 execute interval")
	addr := flag.String("addr", ":9103", "exporter addr")
	flag.Parse()

	execInterval, err := time.ParseDuration(*interval)
	if err != nil {
		log.Fatal("[FATAL] invalid interval, ", err)
	}

	errorCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "network",
		Subsystem: "iperf3",
		Name:      "error_count",
	}, []string{"server"})
	sentBitPerSec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "network",
		Subsystem: "iperf3",
		Name:      "sent_bits_per_second",
	}, []string{"server"})
	sentRetransmits := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "network",
		Subsystem: "iperf3",
		Name:      "sent_retransmits",
	}, []string{"server"})
	receivedBitPerSec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "network",
		Subsystem: "iperf3",
		Name:      "received_bits_per_second",
	}, []string{"server"})
	receivedRetransmits := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "network",
		Subsystem: "iperf3",
		Name:      "received_retransmits",
	}, []string{"server"})
	go func() {
		for {
			result, err := ExecIPerf3(*server, *port)
			if err != nil {
				log.Println("[ERROR] execute iperf3 with error:", err)
				errorCount.WithLabelValues(*server).Add(1)
				sentBitPerSec.Reset()
				sentRetransmits.Reset()
				receivedBitPerSec.Reset()
				receivedRetransmits.Reset()
				time.Sleep(time.Second * 10)
				continue
			}
			sentBitPerSec.WithLabelValues(*server).Set(result.End.SumSent.BitsPerSecond)
			sentRetransmits.WithLabelValues(*server).Set(result.End.SumSent.Retransmits)
			receivedBitPerSec.WithLabelValues(*server).Set(result.End.SumReceived.BitsPerSecond)
			receivedRetransmits.WithLabelValues(*server).Set(result.End.SumReceived.Retransmits)
			time.Sleep(execInterval)
		}
	}()
	prometheus.MustRegister(sentBitPerSec, sentRetransmits, receivedBitPerSec, receivedRetransmits)
	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("[FATAL] start http server fatal:", err)
	}
}
