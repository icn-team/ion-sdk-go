package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	log "github.com/pion/ion-log"
	sdk "github.com/icn-team/ion-sdk-go"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/icn-team/webrtc/v3"
	"github.com/icn-team/webrtc/v3/pkg/media/ivfwriter"
	"github.com/icn-team/webrtc/v3/pkg/media/oggwriter"
)

const (
	audioFileName = "output.ogg"
	videoFileName = "output.ivf"
)

func main() {
	// parse flag
	var session, addr string
	flag.StringVar(&addr, "addr", "localhost:5551", "ion-sfu grpc addr")
	flag.StringVar(&session, "session", "ion", "join session name")
	flag.Parse()

	connector := sdk.NewConnector(addr)
	rtc, err := sdk.NewRTC(connector)
	if err != nil {
		panic(err)
	}

	rtc.OnTrack = func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Println("Got track")
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				rtcpSendErr := rtc.GetSubTransport().GetPeerConnection().WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if rtcpSendErr != nil {
					fmt.Println(rtcpSendErr)
				}
			}
		}()

		oggFile, err := oggwriter.New(audioFileName, 48000, 2)
		if err != nil {
			panic(err)
		}
		defer oggFile.Close()

		ivfFile, err := ivfwriter.New(videoFileName)
		if err != nil {
			panic(err)
		}
		defer ivfFile.Close()

		codecName := strings.Split(track.Codec().RTPCodecCapability.MimeType, "/")[1]
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), codecName)
		buf := make([]byte, 1400)
		rtpPacket := &rtp.Packet{}
		for {
			n, _, readErr := track.Read(buf)
			if readErr != nil {
				log.Errorf("%v", readErr)
				return
			}

			if err = rtpPacket.Unmarshal(buf[:n]); err != nil {
				panic(err)
			}

			if codecName == "opus" {
				log.Debugf("Got Opus track, saving to disk as output.opus (48 kHz, 2 channels)")

				if err := oggFile.WriteRTP(rtpPacket); err != nil {
					log.Panicf("Error write ogg: %v", err)
				}
			} else if codecName == "vp8" {
				log.Debugf("Got VP8 track, saving to disk as output.ivf")

				if len(rtpPacket.Payload) < 4 {
					log.Debugf("Ignore packet: payload is not large enough to ivf container header, %v\n", rtpPacket)
					continue
				}

				if err := ivfFile.WriteRTP(rtpPacket); err != nil {
					log.Panicf("Error write ivf: %v", err)
				}
			}

		}
	}

	// client join a session
	err = rtc.Join(session, sdk.RandomKey(4))

	// publish file to session if needed
	if err != nil {
		log.Errorf("error: %v", err)
	}

	select {}
}
