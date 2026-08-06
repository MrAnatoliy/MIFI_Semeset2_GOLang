package security

import (
	"bytes"
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// PGPCipher выполняет симметричное PGP-шифрование данных карт
// на общем секрете (парольной фразе) из конфигурации.
type PGPCipher struct {
	passphrase []byte
}

// NewPGPCipher создаёт шифратор на основе парольной фразы.
func NewPGPCipher(passphrase string) *PGPCipher {
	return &PGPCipher{passphrase: []byte(passphrase)}
}

// Encrypt шифрует строку и возвращает бинарное PGP-сообщение.
func (c *PGPCipher) Encrypt(plaintext string) ([]byte, error) {
	if len(c.passphrase) == 0 {
		return nil, errors.New("pgp: пустая парольная фраза")
	}
	var buf bytes.Buffer
	cfg := &packet.Config{DefaultCipher: packet.CipherAES256}

	w, err := openpgp.SymmetricallyEncrypt(&buf, c.passphrase, nil, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte(plaintext)); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decrypt расшифровывает PGP-сообщение обратно в строку.
func (c *PGPCipher) Decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", errors.New("pgp: пустые данные")
	}
	attempted := false
	prompt := func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		// Функция вызывается повторно при неверной фразе - прерываем цикл.
		if attempted {
			return nil, errors.New("pgp: неверная парольная фраза")
		}
		attempted = true
		return c.passphrase, nil
	}

	md, err := openpgp.ReadMessage(bytes.NewReader(ciphertext), nil, prompt, nil)
	if err != nil {
		return "", err
	}
	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
