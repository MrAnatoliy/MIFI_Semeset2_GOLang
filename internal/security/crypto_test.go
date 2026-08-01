package security

import "testing"

func TestPGPRoundTrip(t *testing.T) {
	cipher := NewPGPCipher("test-passphrase")
	original := "4276011234567890"

	encrypted, err := cipher.Encrypt(original)
	if err != nil {
		t.Fatalf("шифрование завершилось ошибкой: %v", err)
	}
	if string(encrypted) == original {
		t.Fatal("данные не были зашифрованы")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("расшифровка завершилась ошибкой: %v", err)
	}
	if decrypted != original {
		t.Fatalf("получено %q, ожидалось %q", decrypted, original)
	}
}

func TestPGPWrongPassphrase(t *testing.T) {
	encrypted, err := NewPGPCipher("right").Encrypt("secret")
	if err != nil {
		t.Fatalf("шифрование завершилось ошибкой: %v", err)
	}
	if _, err := NewPGPCipher("wrong").Decrypt(encrypted); err == nil {
		t.Fatal("расшифровка с неверной фразой должна возвращать ошибку")
	}
}

func TestHMACVerify(t *testing.T) {
	signer := NewHMACSigner("hmac-secret")
	data := "4276011234567890|01/30"

	sig := signer.Compute(data)
	if !signer.Verify(data, sig) {
		t.Error("корректная подпись не прошла проверку")
	}
	if signer.Verify(data+"x", sig) {
		t.Error("подпись изменённых данных не должна проходить проверку")
	}
	if NewHMACSigner("other-secret").Verify(data, sig) {
		t.Error("подпись на другом секрете не должна проходить проверку")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("Passw0rd!")
	if err != nil {
		t.Fatalf("хеширование завершилось ошибкой: %v", err)
	}
	if !CheckPassword(hash, "Passw0rd!") {
		t.Error("верный пароль не прошёл проверку")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("неверный пароль прошёл проверку")
	}
}
