// zaehler: liest Klicks aus dem JetStream-Stream KLICKS und zählt sie in PostgreSQL.
// Version 3: wie Version 2, aber jede Nachricht zählt genau einmal.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Ein Statement für den ganzen Batch. Es merkt sich jede Sequenznummer und zählt nur
// Nachrichten, deren Nummer neu ist. Beides gelingt zusammen oder gar nicht.
const zaehlen = `WITH neu AS (
	INSERT INTO klicks_verarbeitet (seq) SELECT unnest($1::bigint[])
	ON CONFLICT DO NOTHING RETURNING seq)
INSERT INTO klicks (code, anzahl)
	SELECT k.code, count(*) FROM unnest($1::bigint[], $2::text[]) AS k(seq, code)
	JOIN neu USING (seq) GROUP BY k.code
	ON CONFLICT (code) DO UPDATE SET anzahl = klicks.anzahl + excluded.anzahl`

func main() {
	ctx := context.Background()
	batch, _ := strconv.Atoi(env("ZAEHLER_BATCH", "500"))
	pause, _ := time.ParseDuration(env("ZAEHLER_PAUSE", "0s"))
	db, err := pgxpool.New(ctx, env("ZAEHLER_DB", "postgres://kurz:lab-kurz-123@s3:6432/kurz"))
	if err != nil {
		log.Fatal(err)
	}
	nc, err := nats.Connect(env("ZAEHLER_NATS", "nats://kurz:lab-nats-123@s2:4222"),
		nats.Name("zaehler"), nats.MaxReconnects(-1))
	if err != nil {
		log.Fatal(err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal(err)
	}
	// Durable: Der Server merkt sich, bis wohin "zaehler" gelesen und bestätigt hat.
	cons, err := js.CreateOrUpdateConsumer(ctx, "KLICKS", jetstream.ConsumerConfig{
		Durable:   "zaehler",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("zaehler: Batch %d, Pause %s", batch, pause)
	gezaehlt, letzte := 0, time.Now()
	for {
		msgs, err := cons.Fetch(batch, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			log.Printf("fetch: %v", err)
			time.Sleep(time.Second)
			continue
		}
		var seqs []int64
		var codes []string
		var liste []jetstream.Msg
		for m := range msgs.Messages() {
			meta, err := m.Metadata()
			if err != nil {
				continue // ohne Sequenznummer nicht zählen: kommt nach AckWait wieder
			}
			seqs = append(seqs, int64(meta.Sequence.Stream))
			codes = append(codes, string(m.Data()))
			liste = append(liste, m)
		}
		if len(liste) == 0 {
			continue
		}
		if _, err := db.Exec(ctx, zaehlen, seqs, codes); err != nil {
			log.Printf("zählen: %v", err) // nicht bestätigt: JetStream liefert sie erneut
			continue
		}
		time.Sleep(pause) // nur für Drill 7: eine Lücke zwischen Commit und Bestätigung
		for _, m := range liste {
			m.Ack()
		}
		gezaehlt += len(liste)
		if time.Since(letzte) >= 5*time.Second {
			log.Printf("gezählt %d", gezaehlt)
			// Bis ack_floor ist alles bestätigt und kommt nie wieder: Nummern wegräumen
			if info, err := cons.Info(ctx); err == nil {
				db.Exec(ctx, "DELETE FROM klicks_verarbeitet WHERE seq <= $1",
					int64(info.AckFloor.Stream))
			}
			letzte = time.Now()
		}
	}
}
