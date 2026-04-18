package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ampmanager/internal/mockresponses"

	"nhooyr.io/websocket"
)

func main() {
	cfg := mockresponses.DefaultSimulationConfig()
	listenAddr := ":8099"

	flag.StringVar(&listenAddr, "listen", listenAddr, "listen address")
	flag.DurationVar(&cfg.ChunkInterval, "chunk-interval", cfg.ChunkInterval, "stream chunk interval")
	flag.DurationVar(&cfg.TTFBMin, "ttfb-min", cfg.TTFBMin, "minimum TTFB")
	flag.DurationVar(&cfg.TTFBMainMax, "ttfb-main-max", cfg.TTFBMainMax, "main TTFB upper bound")
	flag.DurationVar(&cfg.TTFBTailMax, "ttfb-tail-max", cfg.TTFBTailMax, "tail TTFB upper bound")
	flag.Float64Var(&cfg.TPSMin, "tps-min", cfg.TPSMin, "minimum streamed tokens per second")
	flag.Float64Var(&cfg.TPSMax, "tps-max", cfg.TPSMax, "maximum streamed tokens per second")
	flag.DurationVar(&cfg.OutputMin, "output-min", cfg.OutputMin, "minimum output duration")
	flag.DurationVar(&cfg.OutputMax, "output-max", cfg.OutputMax, "maximum output duration")
	flag.IntVar(&cfg.DefaultInputTokens, "default-input-tokens", cfg.DefaultInputTokens, "fallback input token estimate")
	flag.Parse()

	if err := validateConfig(cfg); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/v1/responses", handleResponses(cfg))

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       5 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("fake llm proxy listening on %s", listenAddr)
	log.Printf("supported models: %v", mockresponses.SupportedModels())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	data := make([]map[string]any, 0, len(mockresponses.SupportedModels()))
	for _, modelName := range mockresponses.SupportedModels() {
		data = append(data, map[string]any{
			"id":       modelName,
			"object":   "model",
			"created":  0,
			"owned_by": "fake-llm-proxy",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   data,
	})
}

func handleResponses(cfg mockresponses.SimulationConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isWebsocketUpgrade(r) {
			handleResponsesWebsocket(w, r, cfg)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			mockresponses.WriteHTTPError(w, http.StatusBadRequest, "failed to read request body")
			return
		}

		profile, err := mockresponses.NewAutoProfile(body, cfg, nil)
		if err != nil {
			mockresponses.WriteHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := mockresponses.SleepContext(r.Context(), profile.TTFB); err != nil {
			return
		}
		if profile.Request.Stream {
			if err := mockresponses.StreamSSE(r.Context(), w, profile); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("sse stream failed: %v", err)
			}
			return
		}
		if err := mockresponses.SleepContext(r.Context(), profile.OutputDuration); err != nil {
			return
		}
		if err := mockresponses.WriteJSONResponse(w, profile, false); err != nil {
			log.Printf("write json response failed: %v", err)
		}
	}
}

func handleResponsesWebsocket(w http.ResponseWriter, r *http.Request, cfg mockresponses.SimulationConfig) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(8 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				return
			default:
				log.Printf("websocket read failed: %v", err)
				return
			}
		}

		profile, err := mockresponses.NewAutoProfile(payload, cfg, nil)
		if err != nil {
			_ = mockresponses.WriteWebsocketError(context.Background(), conn, http.StatusBadRequest, err.Error())
			continue
		}

		switch profile.Request.RequestType {
		case "", "response.create", "response.append":
		default:
			_ = mockresponses.WriteWebsocketError(context.Background(), conn, http.StatusBadRequest, "unsupported websocket request type")
			continue
		}

		if err := mockresponses.SleepContext(ctx, profile.TTFB); err != nil {
			return
		}
		if err := mockresponses.StreamWebsocket(ctx, conn, profile); err != nil {
			switch websocket.CloseStatus(err) {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				return
			default:
				log.Printf("websocket stream failed: %v", err)
				return
			}
		}
	}
}

func isWebsocketUpgrade(r *http.Request) bool {
	return http.MethodGet == r.Method &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func validateConfig(cfg mockresponses.SimulationConfig) error {
	switch {
	case cfg.DefaultInputTokens <= 0:
		return errors.New("default-input-tokens must be > 0")
	case cfg.ChunkInterval <= 0:
		return errors.New("chunk-interval must be > 0")
	case cfg.TTFBMin <= 0:
		return errors.New("ttfb-min must be > 0")
	case cfg.TTFBMin > cfg.TTFBMainMax:
		return errors.New("ttfb-min must be <= ttfb-main-max")
	case cfg.TTFBMainMax > cfg.TTFBTailMax:
		return errors.New("ttfb-main-max must be <= ttfb-tail-max")
	case cfg.TPSMin <= 0 || cfg.TPSMin > cfg.TPSMax:
		return errors.New("tps range is invalid")
	case cfg.OutputMin <= 0 || cfg.OutputMin > cfg.OutputMax:
		return errors.New("output duration range is invalid")
	default:
		return nil
	}
}
