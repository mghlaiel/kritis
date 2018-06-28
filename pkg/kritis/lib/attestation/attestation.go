/*
Copyright 2018 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package attestation

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/openpgp"

	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/clearsign"
	"golang.org/x/crypto/openpgp/packet"
)

func VerifyImageAttestation(publicKeyStr string, attestationHash string) error {
	attestation, err := base64.StdEncoding.DecodeString(attestationHash)
	if err != nil {
		return err
	}

	key, err := parsePublicKey(publicKeyStr)
	if err != nil {
		return err
	}
	b, _ := clearsign.Decode(attestation)

	reader := packet.NewReader(b.ArmoredSignature.Body)
	pkt, err := reader.Next()
	if err != nil {
		return err
	}

	sig, ok := pkt.(*packet.Signature)
	if !ok {
		return fmt.Errorf("Not a valid signature")
	}

	hash := sig.Hash.New()
	io.Copy(hash, bytes.NewReader(b.Bytes))

	err = key.VerifySignature(hash, sig)
	if err != nil {
		return err
	}
	return nil
}

func AttestImageSignature(pubKey *packet.PublicKey, privKey *packet.PrivateKey, message io.Reader) (string, error) {
	signer := createEntityFromKeys(pubKey, privKey)
	var b bytes.Buffer
	err := openpgp.ArmoredDetachSign(&b, signer, message, nil)
	return base64.StdEncoding.EncodeToString(b.Bytes()), err
}

func createEntityFromKeys(pubKey *packet.PublicKey, privKey *packet.PrivateKey) *openpgp.Entity {
	config := packet.Config{
		DefaultHash:            crypto.SHA256,
		DefaultCipher:          packet.CipherAES256,
		DefaultCompressionAlgo: packet.CompressionZLIB,
		CompressionConfig: &packet.CompressionConfig{
			Level: 9,
		},
		RSABits: 4096,
	}
	currentTime := config.Now()
	uid := packet.NewUserId("", "", "")

	e := openpgp.Entity{
		PrimaryKey: pubKey,
		PrivateKey: privKey,
		Identities: make(map[string]*openpgp.Identity),
	}
	isPrimaryId := false

	e.Identities[uid.Id] = &openpgp.Identity{
		Name:   uid.Name,
		UserId: uid,
		SelfSignature: &packet.Signature{
			CreationTime: currentTime,
			SigType:      packet.SigTypePositiveCert,
			PubKeyAlgo:   packet.PubKeyAlgoRSA,
			Hash:         config.Hash(),
			IsPrimaryId:  &isPrimaryId,
			FlagsValid:   true,
			FlagSign:     true,
			FlagCertify:  true,
			IssuerKeyId:  &e.PrimaryKey.KeyId,
		},
	}

	keyLifetimeSecs := uint32(86400 * 365)

	e.Subkeys = make([]openpgp.Subkey, 1)
	e.Subkeys[0] = openpgp.Subkey{
		PublicKey:  pubKey,
		PrivateKey: privKey,
		Sig: &packet.Signature{
			CreationTime:              currentTime,
			SigType:                   packet.SigTypeSubkeyBinding,
			PubKeyAlgo:                packet.PubKeyAlgoRSA,
			Hash:                      config.Hash(),
			PreferredHash:             []uint8{8}, // SHA-256
			FlagsValid:                true,
			FlagEncryptStorage:        true,
			FlagEncryptCommunications: true,
			IssuerKeyId:               &e.PrimaryKey.KeyId,
			KeyLifetimeSecs:           &keyLifetimeSecs,
		},
	}
	return &e
}

func parsePublicKey(publicKey string) (*packet.PublicKey, error) {
	pkt, err := parseKey(publicKey, openpgp.PublicKeyType)
	if err != nil {
		return nil, err
	}
	key, ok := pkt.(*packet.PublicKey)
	if !ok {
		return nil, fmt.Errorf("Not a public key")
	}
	return key, nil
}

func parsePrivateKey(privateKey string) (*packet.PrivateKey, error) {
	pkt, err := parseKey(privateKey, openpgp.PrivateKeyType)
	if err != nil {
		return nil, err
	}
	key, ok := pkt.(*packet.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Not a private Key")
	}
	return key, nil
}

func parseKey(key string, keytype string) (packet.Packet, error) {
	f, err := os.Open(key)
	if err != nil {
		return nil, err
	}
	block, err := armor.Decode(f)
	if err != nil {
		return nil, err
	}
	if block.Type != keytype {
		return nil, err
	}
	reader := packet.NewReader(block.Body)
	return reader.Next()
}
