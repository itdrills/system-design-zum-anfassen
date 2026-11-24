package main

import (
	"context"
	"errors"
	"hash/crc32"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// shards[0] ist db. KURZ_SHARDS nennt weitere Datenbanken, durch Leerzeichen getrennt.
var shards []*pgxpool.Pool

func initShards() {
	shards = []*pgxpool.Pool{db}
	for _, dsn := range strings.Fields(env("KURZ_SHARDS", "")) {
		p, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			log.Fatal(err)
		}
		shards = append(shards, p)
	}
	log.Printf("%s: %d Shard(s)", instanz, len(shards))
}

// shardFuer: Welcher Shard einen Link hält, entscheidet allein sein Code.
// Dieselbe Rechnung in SQL: crc32(code::bytea) % Anzahl der Shards.
func shardFuer(code string) int {
	return int(crc32.ChecksumIEEE([]byte(code)) % uint32(len(shards)))
}

// sucheURL fragt jeden Shard nach der Adresse. Sie ist nicht der Shard-Schlüssel,
// also kann sie auf jedem Shard liegen.
func sucheURL(ctx context.Context, url string) (string, error) {
	var code string
	for _, s := range shards {
		err := verbindung(ctx, s).QueryRow(ctx,
			"SELECT code FROM links WHERE url = $1 LIMIT 1", url).Scan(&code)
		if !errors.Is(err, pgx.ErrNoRows) {
			return code, err
		}
	}
	return "", pgx.ErrNoRows
}
