package main

import (
	"fmt"
)

func main() {
	torControl := &TorControl{}

	torControlNetwork := "tcp"
	// your tor control port is usually 9051
	torControlAddr := "127.0.0.1:9951"
	// set this to your tor control port authentication password
	torControlAuthPassword := "toositai8uRupohnugiCeekiex5phahx"
	secretServiceDir := "/var/lib/tor-alpha/hiddenService"
	secretServicePort := map[int]string{80: "127.0.0.1:80"}

	var err error = nil
	err = torControl.Dial(torControlNetwork, torControlAddr)
	if err != nil {
		fmt.Print("connect fail\n")
		return
	}

	err = torControl.PasswordAuthenticate(torControlAuthPassword)
	if err != nil {
		fmt.Print("Tor control port password authentication fail\n")
		return
	}
	fmt.Print("Tor control port password authentication successful.\n")

	err = torControl.CreateHiddenService(secretServiceDir, secretServicePort)
	if err != nil {
		fmt.Printf("create hidden service fail: %s\n", err)
		return
	}
	fmt.Print("Tor hidden service created.\n")

	// XXX
	onion := ""
	onion, err = ReadOnion(secretServiceDir)
	if err != nil {
		fmt.Printf("ReadOnion error: %s\n", err)
		return
	}
	fmt.Printf("hidden service onion: %s\n", onion)

}
