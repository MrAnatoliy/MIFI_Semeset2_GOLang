package security

import "testing"

func TestGenerateCardNumberIsValidLuhn(t *testing.T) {
	for i := 0; i < 200; i++ {
		number, err := GenerateCardNumber()
		if err != nil {
			t.Fatalf("генерация номера завершилась ошибкой: %v", err)
		}
		if len(number) != 16 {
			t.Fatalf("ожидалось 16 цифр, получено %d (%s)", len(number), number)
		}
		if !ValidLuhn(number) {
			t.Fatalf("номер %s не проходит проверку по алгоритму Луна", number)
		}
	}
}

func TestValidLuhnKnownValues(t *testing.T) {
	cases := map[string]bool{
		"4539578763621486": true,
		"79927398713":      true,
		"79927398710":      false,
		"1234567812345678": false,
		"":                 false,
		"12a4":             false,
	}
	for number, want := range cases {
		if got := ValidLuhn(number); got != want {
			t.Errorf("ValidLuhn(%q) = %v, ожидалось %v", number, got, want)
		}
	}
}

func TestMaskCardNumber(t *testing.T) {
	masked := MaskCardNumber("4276011234567890")
	if masked != "4276 **** **** 7890" {
		t.Errorf("неожиданная маска: %s", masked)
	}
}

func TestExpiryValid(t *testing.T) {
	if !ExpiryValid(GenerateExpiry()) {
		t.Error("свежесгенерированный срок действия должен быть валиден")
	}
	if ExpiryValid("01/20") {
		t.Error("истёкший срок действия не должен считаться валидным")
	}
}

func TestGenerateAccountNumberLength(t *testing.T) {
	number, err := GenerateAccountNumber()
	if err != nil {
		t.Fatalf("ошибка генерации номера счёта: %v", err)
	}
	if len(number) != 20 {
		t.Fatalf("ожидалось 20 цифр, получено %d (%s)", len(number), number)
	}
}
