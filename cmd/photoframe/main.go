// Command photoframe runs an e-ink photo frame on a Raspberry Pi with a
// Pimoroni Inky Impression 7.3" panel, plus the web app that configures it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chinmay28/photoframe/internal/api"
	"github.com/chinmay28/photoframe/internal/buttons"
	"github.com/chinmay28/photoframe/internal/config"
	"github.com/chinmay28/photoframe/internal/devdisplay"
	"github.com/chinmay28/photoframe/internal/inky"
	"github.com/chinmay28/photoframe/internal/library"
	"github.com/chinmay28/photoframe/internal/player"
	"github.com/chinmay28/photoframe/internal/source"
	"github.com/chinmay28/photoframe/internal/sysinfo"
	"github.com/chinmay28/photoframe/internal/thumbs"
	"github.com/chinmay28/photoframe/internal/version"
	"github.com/chinmay28/photoframe/web"
)

type options struct {
	dataDir      string
	listen       string
	display      string
	pngPath      string
	pngDelay     time.Duration
	spiDev       string
	gpioChip     string
	buttons      bool
	syncInterval time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.dataDir, "data", "/var/lib/photoframe", "directory for settings, uploads and caches")
	flag.StringVar(&o.listen, "listen", ":8080", "HTTP listen address for the app")
	flag.StringVar(&o.display, "display", "inky", `"inky" for the panel, "png" to write frames to a file`)
	flag.StringVar(&o.pngPath, "png", "", "output file for -display=png (default <data>/display.png)")
	flag.DurationVar(&o.pngDelay, "png-delay", 2*time.Second, "simulated refresh time for -display=png")
	flag.StringVar(&o.spiDev, "spi", "/dev/spidev0.0", "SPI device for the panel")
	flag.StringVar(&o.gpioChip, "gpiochip", "gpiochip0", "GPIO chip for the panel and buttons")
	flag.BoolVar(&o.buttons, "buttons", true, "react to the A-D buttons (inky display only)")
	flag.DurationVar(&o.syncInterval, "sync-interval", 15*time.Minute, "how often to re-sync photo sources")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("photoframe", version.String())
		return
	}
	if err := run(o); err != nil {
		log.Fatal(err)
	}
}

// panel is what both the real panel and the PNG stand-in provide.
type panel interface {
	player.Display
	SetModel(*inky.Model)
}

func run(o options) error {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if err := os.MkdirAll(o.dataDir, 0o755); err != nil {
		return err
	}
	store, err := config.Open(filepath.Join(o.dataDir, "settings.json"))
	if err != nil {
		return err
	}
	model, err := inky.ModelFor(store.Get().Display.Panel)
	if err != nil {
		return err
	}

	disp, closeDisp, err := openDisplay(o, model)
	if err != nil {
		return err
	}
	defer closeDisp()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	factory := source.Factory{DataDir: o.dataDir}
	lib := library.New(factory.New)
	lib.Configure(store.Get().Sources)

	ready := make(chan struct{})
	go func() {
		lib.SyncAll(ctx)
		close(ready)
		lib.Run(ctx, o.syncInterval)
	}()

	// Apply settings changes that affect sources and the panel model.
	settingsChanged := store.Subscribe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-settingsChanged:
				s := store.Get()
				lib.Configure(s.Sources)
				lib.Kick()
				if m, err := inky.ModelFor(s.Display.Panel); err == nil {
					disp.SetModel(m)
				}
			}
		}
	}()

	p := player.New(player.Deps{
		Display:         disp,
		Library:         lib,
		Settings:        store.Get,
		SettingsChanged: store.Subscribe(),
		Ready:           ready,
		StatePath:       filepath.Join(o.dataDir, "state.json"),
		InfoLines:       func() []string { return infoLines(o.listen) },
		Logger:          logger,
	})
	playerDone := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(playerDone)
	}()

	if o.display == "inky" && o.buttons {
		closeButtons, err := buttons.Watch(o.gpioChip, func(i int) {
			cmds := [4]player.Command{player.Next, player.Prev, player.Pause, player.Info}
			c := cmds[i]
			if c == player.Pause && p.Status().Paused {
				c = player.Resume
			}
			logger.Printf("button %c: %s", 'A'+i, c)
			p.Do(c)
		})
		if err != nil {
			logger.Printf("buttons disabled: %v", err)
		} else {
			defer closeButtons()
		}
	}

	srv := &http.Server{
		Addr: o.listen,
		Handler: (&api.Server{
			Store:     store,
			Library:   lib,
			Player:    p,
			Thumbs:    thumbs.New(filepath.Join(o.dataDir, "thumbs"), 320),
			UploadDir: factory.UploadDir,
			Device:    sysinfo.Reader{Version: version.String()}.Read,
			Restart:   stop, // exits cleanly; systemd starts us again
			Static:    web.FS(),
			Logger:    logger,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Printf("photoframe %s: app on %s, display %s", version.String(), o.listen, o.display)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("http: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Printf("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	<-playerDone // the panel is powered off before we exit
	return nil
}

func openDisplay(o options, m *inky.Model) (panel, func() error, error) {
	switch o.display {
	case "inky":
		hw, closeHW, err := inky.OpenLinux(o.spiDev, o.gpioChip)
		if err != nil {
			return nil, nil, fmt.Errorf("open panel: %w", err)
		}
		return inky.New(hw, m), closeHW, nil
	case "png":
		path := o.pngPath
		if path == "" {
			path = filepath.Join(o.dataDir, "display.png")
		}
		return devdisplay.New(path, m, o.pngDelay), func() error { return nil }, nil
	}
	return nil, nil, fmt.Errorf("unknown -display %q", o.display)
}

// infoLines is the text on the welcome and info screens: where to find the app.
func infoLines(listen string) []string {
	port := ""
	if _, p, err := net.SplitHostPort(listen); err == nil && p != "80" && p != "" {
		port = ":" + p
	}
	host, _ := os.Hostname()
	lines := []string{"Open the app to add photos and change settings:", ""}
	if host != "" {
		lines = append(lines, "http://"+strings.ToLower(host)+".local"+port)
	}
	for _, addr := range (sysinfo.Reader{}).Read().Addresses {
		lines = append(lines, "http://"+addr+port)
	}
	return lines
}

// Compile-time checks that both displays satisfy the interface.
var (
	_ panel = (*inky.Device)(nil)
	_ panel = (*devdisplay.PNG)(nil)
)
