package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

type CPUTempObj struct {
	TimeStamp   time.Time
	HostAddress string
	CPUTemp     float64
}

// A single Broker will be created in this program. It is responsible
// for keeping a list of which clients are currently attached
// and broadcasting events (messages) to those clients.
//
type Broker struct {

	// Create a map of clients, the keys of the map are the channels
	// over which we can push CPU temperature to attached clients.  (The values
	// are just booleans and are meaningless.)
	//
	clients map[chan []byte]bool

	// Channel into which new clients can be pushed
	//
	newClients chan chan []byte

	// Channel into which disconnected clients should be pushed
	//
	defunctClients chan chan []byte

	// Channel into which CPU temperature are pushed to be broadcast out
	// to attahed clients.
	//
	tempData chan []byte
}

func GetNodeIPAddress() string {

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("could not obtain host IP address: %v", err)
	}
	ip := ""
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip = ipnet.IP.String()
				break
			}
		}
	}

	return ip
}

// CPU temperatures
func GetCPUTemp() []byte {
	hostIP := GetNodeIPAddress()
	log.Println("CPU temperature")

	// Its a mockup CPU temperature
	cpuTempObj := new(CPUTempObj)
	cpuTempObj.TimeStamp = time.Now()
	cpuTempObj.HostAddress = hostIP
	cpuTempObj.CPUTemp = 38.25

	jsonObj, err := json.Marshal(cpuTempObj)
	if err != nil {
		log.Println(fmt.Sprintf("Could not marshal the response data: %v", err))
	}
	return jsonObj

}

// This Broker method starts a new goroutine.  It handles
// the addition & removal of clients, as well as the broadcasting
// of CPU temperature out to clients that are currently attached. In this case, there will be one client per node.

//
func (b *Broker) Listen() {

	// Start a goroutine
	//
	go func() {

		// Loop endlessly
		//
		for {

			// Block until we receive from one of the
			// three following channels.
			select {

			case s := <-b.newClients:

				// There is a new client attached and we
				// want to start sending them messages.
				b.clients[s] = true
				log.Println("Added new client")

			case s := <-b.defunctClients:

				// A client has dettached and we want to
				// stop sending them messages.
				delete(b.clients, s)
				close(s)

				log.Println("Removed client")

			case msg := <-b.tempData:

				// There is a new message to send.  For each
				// attached client, push the new message
				// into the client's message channel.
				for s := range b.clients {
					s <- msg
				}
				log.Printf("Broadcast message to %d clients", len(b.clients))
			}
		}
	}()
}

// This Broker method handles and HTTP request at the "/events/" URL.
//
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Make sure that the writer supports flushing.
	//
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Create a new channel, over which the broker can
	// send this client messages.
	messageChan := make(chan []byte)

	// Add this client to the map of those that should
	// receive updates
	b.newClients <- messageChan

	// Listen to the closing of the http connection via the CloseNotifier
	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		// Remove this client from the map of attached clients
		// when `EventHandler` exits.
		b.defunctClients <- messageChan
		log.Println("HTTP connection just closed.")
	}()

	// Set the headers related to event streaming.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Don't close the connection, instead loop endlessly.
	for {

		// Read from our messageChan.
		msg, open := <-messageChan

		if !open {
			// If our messageChan was closed, this means that the client has
			// disconnected.
			break
		}

		// Write to the ResponseWriter, `w`.
		fmt.Fprintf(w, "CPU Temperature: %s\n\n", msg)

		// Flush the response.  This is only possible if
		// the repsonse supports streaming.
		f.Flush()
	}

	// Done.
	log.Println("Finished HTTP request at ", r.URL.Path)
}

// Main routine
//
func main() {

	// Make a new Broker instance
	b := &Broker{
		make(map[chan []byte]bool),
		make(chan (chan []byte)),
		make(chan (chan []byte)),
		make(chan []byte),
	}

	// Start processing events
	b.Listen()

	// Make b the HTTP handler for "/redfish/v1/Chassis/<chassis-id>/Thermal".  It can do
	// this because it has a ServeHTTP method.  That method
	// is called in a separate goroutine for each
	// request to "/redfish/v1/Chassis/<chassis-id>/Thermal/".
	http.Handle("/redfish/v1/Systems/1/Processors/Power", b)
	http.Handle("/redfish/v1/Systems/1/Memory/Power", b)

	//tickIntervav := "10"
	//dur, _ := time.ParseDuration(tickIntervav)
	//ticker := time.NewTicker(10 * time.)
	ticker := time.NewTicker(2 * time.Second)

	// Generate a constant stream of events that get pushed
	// into the Broker's messages channel and are then broadcast
	// out to any clients that are attached.
	go func() {
		for i := 0; ; i++ {

			select {
			case <-ticker.C:
				b.tempData <- GetCPUTemp()
				// Print log message
				log.Printf("Sequence#: %d, Temperature: %v ", i, b.tempData)
			}
		}
	}()

	hostIP := GetNodeIPAddress()
	log.Println("Starting server...", hostIP)
	// Start the server and listen forever on port 8000.
	http.ListenAndServe("10.0.75.74:8000", nil)

}
