package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// serve beantwortet Anfragen, bis SIGTERM kommt (systemctl stop/restart),
// und beantwortet dann die laufenden Anfragen noch fertig.
func serve(handler http.Handler) {
	srv := &http.Server{Addr: env("KURZ_ADDR", ":8080"), Handler: handler}
	go func() {
		log.Printf("%s lauscht auf %s", instanz, srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	log.Printf("%s fährt herunter", instanz)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Shutdown: %v", err)
	}
	db.Close()
}
