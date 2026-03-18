package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	log := utils.GetLogger()

	brokers := strings.TrimSpace(os.Getenv("GEMINI_KAFKA_BROKERS"))
	topic := strings.TrimSpace(os.Getenv("GEMINI_KAFKA_TOPIC"))
	group := strings.TrimSpace(os.Getenv("GEMINI_KAFKA_GROUP"))
	user := strings.TrimSpace(os.Getenv("GEMINI_KAFKA_USERNAME"))
	pass := strings.TrimSpace(os.Getenv("GEMINI_KAFKA_PASSWORD"))
	if brokers == "" || topic == "" || group == "" || user == "" || pass == "" {
		log.Fatal("set GEMINI_KAFKA_BROKERS, GEMINI_KAFKA_TOPIC, GEMINI_KAFKA_GROUP, GEMINI_KAFKA_USERNAME, GEMINI_KAFKA_PASSWORD")
	}

	d := &kafka.Dialer{
		Timeout:       30 * time.Second,
		DualStack:     true,
		SASLMechanism: plain.Mechanism{Username: user, Password: pass},
		TLS:           &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GEMINI_KAFKA_INSECURE_SKIP_VERIFY")), "true") {
		d.TLS.InsecureSkipVerify = true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GEMINI_KAFKA_TLS")), "false") {
		d.TLS = nil
	}

	start := kafka.LastOffset
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GEMINI_KAFKA_START")), "earliest") {
		start = kafka.FirstOffset
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     splitBrokers(brokers),
		GroupID:     group,
		Topic:       topic,
		Dialer:      d,
		StartOffset: start,
		MinBytes:    1,
		MaxBytes:    50e6,
		MaxWait:     5 * time.Second,
	})
	defer r.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Infow("gemini firehose", "topic", topic, "group", group)

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warnw("fetch", "error", err)
			time.Sleep(time.Second)
			continue
		}

		parsed, err := parseConsistMessage(msg.Value)
		if err != nil {
			log.Warnw("parse", "error", err, "partition", msg.Partition, "offset", msg.Offset)
		} else {
			logConsist(parsed)
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Warnw("commit", "error", err)
		}
	}
}

func parseConsistMessage(xmlBody []byte) (*types.PassengerTrainConsistMessage, error) {
	var msg types.PassengerTrainConsistMessage
	if err := xml.Unmarshal(xmlBody, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func tiplocFrom(loc *types.GeminiLocationIdent) string {
	if loc == nil || loc.LocationSubsidiaryIdentification == nil {
		return ""
	}
	return strings.TrimSpace(loc.LocationSubsidiaryIdentification.LocationSubsidiaryCode.Text)
}

func logConsist(msg *types.PassengerTrainConsistMessage) {
	op := strings.TrimSpace(msg.OperationalTrainNumberIdentifier.OperationalTrainNumber)
	core, start := "", ""
	if t := msg.TrainOperationalIdentification.Transports; len(t) > 0 {
		core = strings.TrimSpace(t[0].Core)
		start = strings.TrimSpace(t[0].StartDate)
	}
	if len(msg.Allocations) == 0 {
		fmt.Printf("OperationalTrainNumber=%s core=%s start_date=%s ResourceGroupId= vehicles=0 origin= dest=\n", op, core, start)
		return
	}
	for _, a := range msg.Allocations {
		rg := a.ResourceGroup
		fmt.Printf("OperationalTrainNumber=%s core=%s start_date=%s ResourceGroupId=%s type=%s vehicles=%d vehicle_ids=%s origin=%s dest=%s\n",
			op, core, start,
			strings.TrimSpace(rg.ResourceGroupId),
			strings.TrimSpace(rg.TypeOfResource),
			len(rg.Vehicles),
			formatVehicles(rg.Vehicles),
			tiplocFrom(a.TrainOriginLocation),
			tiplocFrom(a.TrainDestLocation),
		)
	}
}

// formatVehicles lists each VehicleId with type (L=loco, C=coach) so HST power cars are visible alongside set id (ResourceGroupId).
func formatVehicles(vs []types.GeminiVehicle) string {
	if len(vs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, v := range vs {
		if i > 0 {
			b.WriteByte(',')
		}
		id := strings.TrimSpace(v.VehicleId)
		t := strings.TrimSpace(v.TypeOfVehicle)
		if t != "" {
			fmt.Fprintf(&b, "%s(%s)", id, t)
		} else {
			b.WriteString(id)
		}
	}
	return b.String()
}

func splitBrokers(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
