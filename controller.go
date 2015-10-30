/*
 * controller.go - goControlTor
 * Copyright (C) 2014  Yawning Angel, David Stainton
 * Copyright (C) 2015  Guido Witmond (Epemeral additions)
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package goControlTor

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/textproto"
	"path"
	"regexp"
	"strings"
)

const (
	cmdOK            = 250
	cmdAuthenticate  = "AUTHENTICATE"
	cmdAuthChallenge = "AUTHCHALLENGE"
	//	authMethodCookie     = "COOKIE"
	//	authMethodNull       = "NULL"

	respAuthChallenge = "AUTHCHALLENGE "

	argServerHash  = "SERVERHASH="
	argServerNonce = "SERVERNONCE="

	authMethodSafeCookie = "SAFECOOKIE"
	authNonceLength      = 32

	authServerHashKey = "Tor safe cookie authentication server-to-controller hash"
	authClientHashKey = "Tor safe cookie authentication controller-to-server hash"
)

type TorControl struct {
	controlConn            net.Conn
	textprotoReader        *textproto.Reader
	authenticationPassword string
}

// Dial handles unix domain sockets and tcp!
func (t *TorControl) Dial(network, addr string) error {
	var err error = nil
	t.controlConn, err = net.Dial(network, addr)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(t.controlConn)
	t.textprotoReader = textproto.NewReader(reader)
	return nil
}

func (t *TorControl) SendCommand(command string, expect int) (int, string, error) {
	var code int
	var message string
	var err error

	_, err = t.controlConn.Write([]byte(command))
	if err != nil {
		return 0, "", fmt.Errorf("writing to tor control port: %s", err)
	}
	code, message, err = t.textprotoReader.ReadResponse(expect)
	return code, message, nil
}

func (t *TorControl) SafeCookieAuthenticate(cookiePath string) error {

	var code int
	var message string

	cookie, err := readAuthCookie(cookiePath)
	if err != nil {
		return err
	}

	cookie, err = t.authSafeCookie(cookie)
	if err != nil {
		return err
	}
	cookieStr := hex.EncodeToString(cookie)
	authReq := fmt.Sprintf("%s %s\n", cmdAuthenticate, cookieStr)

	code, message, err = t.SendCommand(authReq, cmdOK)
	if err != nil {
		return fmt.Errorf("Safe Cookie Authentication fail: %s %s %s", code, message, err)
	}

	return nil
}

func (t *TorControl) CookieAuthenticate(cookiePath string) error {
	var code int
	var message string

	cookie, err := readAuthCookie(cookiePath)
	if err != nil {
		return err
	}

	cookieStr := hex.EncodeToString(cookie)
	authReq := fmt.Sprintf("%s %s\n", cmdAuthenticate, cookieStr)

	code, message, err = t.SendCommand(authReq, cmdOK)
	if err != nil {
		return fmt.Errorf("Cookie Authentication fail: %s %s %s", code, message, err)
	}

	return nil
}

func readAuthCookie(path string) ([]byte, error) {
	// Read the cookie auth file.
	cookie, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cookie auth file: %s", err)
	}
	return cookie, nil
}

func (t *TorControl) authSafeCookie(cookie []byte) ([]byte, error) {
	var code int
	var message string
	var err error

	clientNonce := make([]byte, authNonceLength)
	if _, err := rand.Read(clientNonce); err != nil {
		return nil, fmt.Errorf("generating AUTHCHALLENGE nonce: %s", err)
	}
	clientNonceStr := hex.EncodeToString(clientNonce)

	// Send and process the AUTHCHALLENGE.
	authChallengeReq := []byte(fmt.Sprintf("%s %s %s\n", cmdAuthChallenge, authMethodSafeCookie, clientNonceStr))
	if _, err := t.controlConn.Write(authChallengeReq); err != nil {
		return nil, fmt.Errorf("writing AUTHCHALLENGE request: %s", err)
	}

	code, message, err = t.textprotoReader.ReadCodeLine(cmdOK)
	if err != nil {
		return nil, fmt.Errorf("reading tor control port command status: %s %s %s", code, message, err)
	}

	lineStr := strings.TrimSpace(message)
	respStr := strings.TrimPrefix(lineStr, respAuthChallenge)
	if respStr == lineStr {
		return nil, fmt.Errorf("parsing AUTHCHALLENGE response")
	}
	splitResp := strings.SplitN(respStr, " ", 2)
	if len(splitResp) != 2 {
		return nil, fmt.Errorf("parsing AUTHCHALLENGE response")
	}
	hashStr := strings.TrimPrefix(splitResp[0], argServerHash)
	serverHash, err := hex.DecodeString(hashStr)
	if err != nil {
		return nil, fmt.Errorf("decoding AUTHCHALLENGE ServerHash: %s", err)
	}
	serverNonceStr := strings.TrimPrefix(splitResp[1], argServerNonce)
	serverNonce, err := hex.DecodeString(serverNonceStr)
	if err != nil {
		return nil, fmt.Errorf("decoding AUTHCHALLENGE ServerNonce: %s", err)
	}

	// Validate the ServerHash.
	m := hmac.New(sha256.New, []byte(authServerHashKey))
	m.Write([]byte(cookie))
	m.Write([]byte(clientNonce))
	m.Write([]byte(serverNonce))
	dervServerHash := m.Sum(nil)
	if !hmac.Equal(serverHash, dervServerHash) {
		return nil, fmt.Errorf("AUTHCHALLENGE ServerHash is invalid")
	}

	// Calculate the ClientHash.
	m = hmac.New(sha256.New, []byte(authClientHashKey))
	m.Write([]byte(cookie))
	m.Write([]byte(clientNonce))
	m.Write([]byte(serverNonce))

	return m.Sum(nil), nil
}

func (t *TorControl) PasswordAuthenticate(password string) error {
	authCmd := fmt.Sprintf("%s \"%s\"\n", cmdAuthenticate, password)
	_, _, err := t.SendCommand(authCmd, cmdOK)
	return err
}

// Creates a Tor Hidden Service with the HiddenServiceDirGroupReadable option
// set so that the service's hostname file will have group read permission set.
// At this time of writing this feature is only available in the alpha version
// of tor. See https://trac.torproject.org/projects/tor/ticket/11291
func (t *TorControl) CreateHiddenService(serviceDir string, listenAddrs map[int]string) error {
	var createCmd string = fmt.Sprintf("SETCONF hiddenservicedir=%s", serviceDir)
	for virtPort, listenAddr := range listenAddrs {
		createCmd += fmt.Sprintf(" hiddenserviceport=\"%d %s\"", virtPort, listenAddr)
	}
	createCmd += " HiddenServiceDirGroupReadable=1\n"
	_, _, err := t.SendCommand(createCmd, cmdOK)
	return err
}

func (t *TorControl) DeleteHiddenService(serviceDir string) error {
	var deleteCmd string = fmt.Sprintf("SETCONF hiddenservicedir=%s\n", serviceDir)
	_, _, err := t.SendCommand(deleteCmd, cmdOK)
	return err
}

func ReadOnion(serviceDir string) (string, error) {
	onion, err := ioutil.ReadFile(path.Join(serviceDir, "hostname"))
	if err != nil {
		return "", fmt.Errorf("reading Tor hidden service hostname file: %s", err)
	}
	return string(onion), nil
}


// Create ephemeral hidden services.
// These run from creation until the Tor service shuts down.
// Starting a ephemeral service returns the onion address and its private key
// To restart a service at a later date, remember the private key and submit it to Tor.

var onionRE = regexp.MustCompile("ServiceID=([a-z2-7=]+)")
var keyRE   = regexp.MustCompile("PrivateKey=RSA1024:([a-zA-Z0-9+/-_=]+)")

// CreateEphemeralHiddenService creates a new hidden service.
func (t *TorControl) CreateEphemeralHiddenService (port, dest string) (string, string, error) {

	// ADD_ONION SP NEW:BEST SP FLAGS=Detach Port=443,127.0.0.1:12345
	// Returns: 250-ServiceID=<onionaddr>CRLF
	//          250-PrivateKey=<keytype>:<KeyBlob>CRLF
	//          250 OK CRLF

	cmd := fmt.Sprintf("ADD_ONION NEW:BEST FLAGS=Detach Port=%s,%s\n", port, dest)
	_, message, err := t.SendCommand(cmd, cmdOK)
	if err != nil {
		return "", "", err
	}

	onionAddress := getFirst(onionRE.FindStringSubmatch(message))
	onionPrivKey := getFirst(keyRE.FindStringSubmatch(message))
	return onionAddress, onionPrivKey, nil
}

// Return the first (not zeroth) string in the array, if not nil
func getFirst(s []string) (string) {
	if s != nil {
		return s[1]
	}
	return ""
}


// RestartEphemeralHiddenService restarts a new hidden service at the tor node
// TODO: check if it already runs at the service (or if it is idempotent)
func (t *TorControl) RestartEphemeralHiddenService (privkey []byte, port, dest string) (string, error) {

	// TODO: check with these calls to see which onions are still/already up
	// GETINFO onions/current
	// GETINFO onions/detached

	// ADD_ONION SP RSA1024:<keyblob> SP FLAGS=Detach Port=443,127.0.0.1:12345
	// Returns: 250-ServiceID=<onionaddr>CRLF
	//          250-PrivateKey=<keytype>:<KeyBlob>CRLF
	//          250 OK CRLF
	// Or
	//          550 Onion address collision

	cmd := fmt.Sprintf("ADD_ONION RSA1024:%s FLAGS=Detach Port=%s,%s\n", string(privkey), port, dest)
	code, message, err := t.SendCommand(cmd, 0)
	if code == 250 || code == 550 {
		onionAddress := getFirst(onionRE.FindStringSubmatch(message))
		// ignore 550 Onion address collision erorrs, treat as success.
		return onionAddress, nil
	} else if err != nil {
		return "", err
	} else {
		// we have a different return code, report it
		return "", errors.New(fmt.Sprintf("%d %s", code, message))
	}
}
