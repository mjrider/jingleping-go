package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

var (
	dstNetFlag  = flag.String("dst-net", "2001:4c08:2028", "the destination network of the ipv6 tree")
	imageFlag   = flag.String("image", "", "the image to ping to the tree")
	xOffFlag    = flag.Int("x", 0, "the x offset to draw the image")
	yOffFlag    = flag.Int("y", 0, "the y offset to draw the image")
	rateFlag    = flag.Int("rate", 100, "how many times to draw the image per second")
	workersFlag = flag.Int("workers", 1, "the number of workers to use")
)

const (
	maxX = 160
	maxY = 120
)

var pingPacket []byte

func worker(ch <-chan *net.IPAddr) {
	c, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		log.Fatalf("could not open ping socket: %s", err)
	}
	log.Printf("starting worker")

	for {
		for a := range ch {
			_, err = c.WriteTo(pingPacket, a)
			if err != nil {
				log.Printf("warning: could not send ping packet: %s", err)
			}
		}
	}
}

func fill(ch chan<- *net.IPAddr, frames [][]*net.IPAddr, delay []time.Duration) {
	for {
		for fidx, frame := range frames {
			// frame clock
			ticker := time.NewTimer(delay[fidx])

			for _, a := range frame {
				ch <- a
			}
			<-ticker.C
		}
	}
}

func makeAddrs(img image.Image, dstNet string, xOff, yOff int) []*net.IPAddr {
	var addrs []*net.IPAddr

	bounds := img.Bounds()
	for y := 0; y < bounds.Dy() && y+yOff < maxY; y++ {
		for x := 0; x < bounds.Dx() && x+xOff < maxX; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 {
				addrs = append(addrs, &net.IPAddr{
					IP: net.ParseIP(fmt.Sprintf("%s:%d:%d:%x:%x:%x", dstNet, x+xOff, y+yOff, r>>8, g>>8, b>>8)),
				})
			}
		}
	}

	return addrs
}

func main() {
	flag.Parse()

	if *imageFlag == "" {
		fmt.Fprintln(os.Stderr, "the image flag must be provided")
		os.Exit(1)
	}

	var delays []time.Duration
	var frames [][]*net.IPAddr
	var qLen int

	{
		var imgs []image.Image

		{
			f, err := os.Open(*imageFlag)
			if err != nil {
				log.Fatalf("could not open image: %s", err)
			}
			defer f.Close()

			imgs, delays, err = decodeImage(f)
			if err != nil {
				log.Fatalf("could not decode image: %s", err)
			}
		}

		for _, img := range imgs {
			addrs := makeAddrs(img, *dstNetFlag, *xOffFlag, *yOffFlag)
			if len(addrs) > qLen {
				qLen = len(addrs)
			}
			frames = append(frames, addrs)
		}
	}

	if delays == nil {
		delays = []time.Duration{time.Second / time.Duration(*rateFlag)}
	}

	log.Printf("queue length: %d", qLen)

	pixCh := make(chan *net.IPAddr, qLen)
	go fill(pixCh, frames, delays)

	for i := 0; i < *workersFlag; i++ {
		go worker(pixCh)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	log.Println("exiting...")
}

// Setup ping packet
func init() {
	var err error

	p := &icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:  0xDEAD,
			Seq: 1,
		},
	}

	pingPacket, err = p.Marshal(nil)
	if err != nil {
		panic(err)
	}
}
