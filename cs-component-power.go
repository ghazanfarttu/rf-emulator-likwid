// cs-component-power.go
// Uses likwid tool to acquire cpu power usage and memory power
// exposes these power metrics to other applications via Redfish like REST API.
// extended by: Ghazanfar Ali  2/13/2020

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type memPower struct {
	Time                    string  `json:"Time"`
	HostAddress             string  `json:"HostAddress"`
	MemoryAveragePowerUsage float64 `json:"MemoryAveragePowerUsage"`
	MemoryCurrentPowerUsage float64 `json:"MemoryCurrentPowerUsage"`
	MemoryMinPowerUsage     float64 `json:"MemoryMinPowerUsage"`
	MemoryMaxPowerUsage     float64 `json:"MemoryMaxPowerUsage"`
}

type cpuPower struct {
	Time                 string  `json:"Time"`
	HostAddress          string  `json:"HostAddress"`
	CPUAveragePowerUsage float64 `json:"CPUAveragePowerUsage"`
	CPUCurrentPowerUsage float64 `json:"CPUCurrentPowerUsage"`
	CPUMinPowerUsage     float64 `json:"CPUMinPowerUsage"`
	CPUMaxPowerUsage     float64 `json:"CPUMaxPowerUsage"`
}

var (
	cpuAvgPwr float64
	cpuCurPwr float64
	cpuMinPwr float64
	cpuMaxPwr float64

	memAvgPwr float64
	memCurPwr float64
	memMinPwr float64
	memMaxPwr float64
)

func init() {

	cpuAvgPwr = 0.0
	cpuCurPwr = 0.0
	cpuMinPwr = 0.0
	cpuMaxPwr = 0.0

	memAvgPwr = 0.0
	memCurPwr = 0.0
	memMinPwr = 0.0
	memMaxPwr = 0.0

}

// main
func main() {

	router := mux.NewRouter()

	router.HandleFunc("/redfish/v1/Systems/1/Processors/Power", GetCPUPwr).Methods(http.MethodGet)
	router.HandleFunc("/redfish/v1/Systems/1/Memory/Power", GetMemPwr).Methods(http.MethodGet)

	// ticker @ 1 second
	ticker := time.NewTicker(1 * time.Second)

	// acquire cpu and memory power usage periodically
	go func() {
		totalMemPwr := 0.0
		totalCpuPwr := 0.0
		counter := 0
		for i := 0; ; i++ {

			select {
			case <-ticker.C:
				GetPower()

				if counter == 0 {
					cpuMinPwr = cpuCurPwr
					memMinPwr = memCurPwr
				}
				counter += 1
				totalMemPwr += memCurPwr
				totalCpuPwr += cpuCurPwr

				if cpuMinPwr > cpuCurPwr {
					cpuMinPwr = cpuCurPwr
				}
				if cpuMaxPwr < cpuCurPwr {
					cpuMaxPwr = cpuCurPwr
				}

				if memMinPwr > memCurPwr {
					memMinPwr = memCurPwr
				}
				if memMaxPwr < memCurPwr {
					memMaxPwr = memCurPwr
				}

				if counter == 60 {
					cpuAvgPwr = totalCpuPwr / float64(counter)

					memAvgPwr = totalMemPwr / float64(counter)
					counter = 0
					totalCpuPwr = 0.0
					totalMemPwr = 0.0

				}

			}
		}
	}()

	hostIP := GetNodeIPAddress()
	port := "8000"

	log.Println("Starting Redfish emulator at IP: ", hostIP, " and Port: ", port)

	log.Fatal(http.ListenAndServe(hostIP+":"+port, router))

}

// checkErr
// Reports any errors.
func checkErr(err error) {

	if err != nil {

		panic(err)

	}

}

// Get DRAM Power usage
func GetMemPwr(w http.ResponseWriter, r *http.Request) {
	mempwr := new(memPower)
	mempwr.Time = time.Now().Format(time.RFC3339)
	mempwr.HostAddress = GetNodeIPAddress()

	mempwr.MemoryCurrentPowerUsage = math.Round(memCurPwr*100) / 100
	mempwr.MemoryAveragePowerUsage = math.Round(memAvgPwr*100) / 100
	mempwr.MemoryMinPowerUsage = math.Round(memMinPwr*100) / 100
	mempwr.MemoryMaxPowerUsage = math.Round(memMaxPwr*100) / 100
	resp, _ := json.Marshal(mempwr)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

// Get CPU Power usage
func GetCPUPwr(w http.ResponseWriter, r *http.Request) {

	cpupwr := new(cpuPower)
	cpupwr.Time = time.Now().Format(time.RFC3339)
	cpupwr.HostAddress = GetNodeIPAddress()

	cpupwr.CPUCurrentPowerUsage = math.Round(cpuCurPwr*100) / 100
	cpupwr.CPUAveragePowerUsage = math.Round(cpuAvgPwr*100) / 100
	cpupwr.CPUMinPowerUsage = math.Round(cpuMinPwr*100) / 100
	cpupwr.CPUMaxPowerUsage = math.Round(cpuMaxPwr*100) / 100

	resp, _ := json.Marshal(cpupwr)
	//fmt.Fprintf(w,resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)

}
func respondWithError(response http.ResponseWriter, statusCode int, msg string) {
	respondWithJSON(response, statusCode, map[string]string{"error": msg})
}

func respondWithJSON(response http.ResponseWriter, statusCode int, data interface{}) {
	result, _ := json.Marshal(data)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(statusCode)
	response.Write(result)
}

func GetPower() {

	bMem := false
	bCpu := false
	cpuPwr := 0.0
	memPwr := 0.0

	out, err := exec.Command("likwid-powermeter", "-s", "1s").Output()
	if err != nil {
		log.Fatal(err)
	}
	strResp := string(out)
	re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
	strList := strings.Split(strResp, "\n")
	for _, s := range strList {
		if strings.Contains(s, "PKG") {
			bCpu = true
		} else if strings.Contains(s, "DRAM") {
			bMem = true
		} else if strings.Contains(s, "Power consumed") {
			//fmt.Println(s)
			sf := re.FindString(s)
			if sf == "" {
				fmt.Println("Find String empty")
			} else {
				//fmt. Println("sf:",sf)
			}
			if bCpu {
				bCpu = false
				val, _ := strconv.ParseFloat(sf, 64)
				cpuPwr += val
			} else if bMem {
				bMem = false
				v, _ := strconv.ParseFloat(sf, 64)
				memPwr += v
			}

		}
	}
	cpuCurPwr = cpuPwr
	memCurPwr = memPwr
	fmt.Println("CPU Power Usage (Watts): ", cpuCurPwr)
	fmt.Println("Memory Power Usage (Watts): ", memCurPwr)
	fmt.Println("")
	fmt.Println(strResp)
	fmt.Println("")
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
