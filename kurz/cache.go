package main

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
	"golang.org/x/sync/singleflight"
)

// cache ist nil, wenn KURZ_CACHE nicht gesetzt ist.
// Dann geht jede Anfrage an PostgreSQL.
var (
	cache    valkey.Client
	cacheTTL = 10 * time.Minute
	laden    singleflight.Group // fasst gleichzeitige Fehltreffer pro Code zusammen
	// Höchstdauer für das Replikat, danach fragt finde den Primärserver
	lesenTimeout, _ = time.ParseDuration(env("KURZ_TIMEOUT_LESEN", "300ms"))
)

func initCache() {
	addr := env("KURZ_CACHE", "")
	if addr == "" {
		return
	}
	if s, err := strconv.Atoi(env("KURZ_CACHE_TTL", "600")); err == nil {
		cacheTTL = time.Duration(s) * time.Second
	}
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
		Password:    env("KURZ_CACHE_PASSWORT", ""),
	})
	if err != nil {
		log.Fatalf("cache: %v", err)
	}
	cache = c
	log.Printf("%s: Cache %s, TTL %s", instanz, addr, cacheTTL)
}

// finde liefert die Zieladresse zu einem Code: erst aus dem Cache, sonst aus der
// Datenbank, und legt sie dann für cacheTTL im Cache ab (Cache-Aside).
func finde(ctx context.Context, code string) (string, error) {
	key := "link:" + code
	if cache != nil {
		ziel, err := cache.Do(ctx, cache.B().Get().Key(key).Build()).ToString()
		if err == nil {
			return ziel, nil
		}
		if !valkey.IsValkeyNil(err) {
			log.Printf("cache get: %v", err) // Cache gestört: weiter mit der Datenbank
		}
	}
	v, err, _ := laden.Do(code, func() (any, error) {
		var ziel string
		if i := shardFuer(code); i > 0 {
			err := shards[i].QueryRow(ctx, "SELECT url FROM links WHERE code = $1", code).Scan(&ziel)
			return ziel, err
		}
		lctx, cancel := context.WithTimeout(ctx, lesenTimeout)
		err := lesen.QueryRow(lctx, "SELECT url FROM links WHERE code = $1", code).Scan(&ziel)
		cancel()
		if err != nil && lesen != db {
			// Replikat kennt den Code (noch) nicht oder ist weg: Primärserver fragen
			err = db.QueryRow(ctx, "SELECT url FROM links WHERE code = $1", code).Scan(&ziel)
		}
		return ziel, err
	})
	if err != nil {
		return "", err
	}
	ziel := v.(string)
	if cache != nil {
		set := cache.B().Set().Key(key).Value(ziel).Ex(streuen(cacheTTL)).Build()
		err := cache.Do(ctx, set).Error()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("cache set: %v", err)
		}
	}
	return ziel, nil
}

// cacheBereit meldet, ob der Cache antwortet. Dann kann die Instanz beliebte Links
// auch bei überlasteter Datenbank weiterleiten und bleibt im Load Balancer.
func cacheBereit(ctx context.Context) bool {
	return cache != nil && cache.Do(ctx, cache.B().Ping().Build()).Error() == nil
}

// streuen verteilt die Ablaufzeiten zufällig auf 80–120 % der TTL, damit Schlüssel,
// die gleichzeitig gefüllt wurden, nicht alle gleichzeitig ablaufen.
func streuen(ttl time.Duration) time.Duration {
	return ttl*8/10 + rand.N(ttl*4/10)
}
