package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"text/template"

	"github.com/pion/webrtc/v2"

	gst "github.com/pion/example-webrtc-applications/internal/gstreamer-src"
	"github.com/pion/example-webrtc-applications/internal/signal"
)

type turnData struct {
	TurnServer   string
	TurnUser     string
	TurnPassword string
}

func main() {
	audioSrc := flag.String("audio-src", "audiotestsrc is-live=true wave=red-noise", "GStreamer audio src")
	videoSrc := flag.String("video-src", "videotestsrc is-live=true pattern=smpte", "GStreamer video src")
	flag.Parse()

	turn := turnData{os.Getenv("TURN_SERVER"), os.Getenv("TURN_USER"), os.Getenv("TURN_PASSWORD")}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Render index page
		t, err := template.New("index.tmpl.html").ParseFiles("index.tmpl.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("%v", err)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if err := t.Execute(w, turn); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("%v", err)
			return
		}
		return
	})

	http.HandleFunc("/webrtc", func(w http.ResponseWriter, r *http.Request) {
		sdp := r.FormValue("sdp")
		if sdp == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "missing form data 'sdp'")
			return
		}
		fmt.Fprintf(w, startStream(audioSrc, videoSrc, sdp, turn))
	})

	fmt.Println("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func startStream(audioSrc, videoSrc *string, offerB64 string, turn turnData) string {

	// Everything below is the pion-WebRTC API! Thanks for using it ❤️.

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
			{
				URLs:           []string{turn.TurnServer},
				Username:       turn.TurnUser,
				Credential:     turn.TurnPassword,
				CredentialType: webrtc.ICECredentialTypePassword,
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Create a audio track
	audioTrack, err := peerConnection.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), "audio", "pion1")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}

	// Create a video track
	videoTrack, err := peerConnection.NewTrack(webrtc.DefaultPayloadTypeH264, rand.Uint32(), "video", "pion2")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}

	// decode the offer
	offer := webrtc.SessionDescription{}
	signal.Decode(offerB64, &offer)

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(signal.Encode(answer))

	// Start pushing buffers on these tracks
	gst.CreatePipeline(webrtc.Opus, []*webrtc.Track{audioTrack}, *audioSrc).Start()
	gst.CreatePipeline("nvenc", []*webrtc.Track{videoTrack}, *videoSrc).Start()

	return signal.Encode(answer)
}
