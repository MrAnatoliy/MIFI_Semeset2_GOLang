package service

import (
	"math"
	"testing"
	"time"
)

func TestAnnuityPayment(t *testing.T) {
	// 100 000 RUB под 12% годовых на 12 месяцев ≈ 8 884,88 RUB/мес.
	got := AnnuityPayment(100000, 12, 12)
	want := 8884.88
	if math.Abs(got-want) > 0.05 {
		t.Errorf("AnnuityPayment = %.2f, ожидалось ≈ %.2f", got, want)
	}
}

func TestAnnuityPaymentZeroRate(t *testing.T) {
	got := AnnuityPayment(12000, 0, 12)
	if math.Abs(got-1000) > 0.01 {
		t.Errorf("при нулевой ставке платёж должен быть 1000, получено %.2f", got)
	}
}

func TestBuildScheduleClosesPrincipal(t *testing.T) {
	principal := 250000.0
	months := 24
	schedule := BuildSchedule(1, principal, 21.5, months, time.Now())

	if len(schedule) != months {
		t.Fatalf("ожидалось %d платежей, получено %d", months, len(schedule))
	}

	var sumPrincipal, sumTotal float64
	for i, p := range schedule {
		if p.PaymentNumber != i+1 {
			t.Errorf("нарушена нумерация платежей: %d", p.PaymentNumber)
		}
		if p.TotalAmount <= 0 {
			t.Errorf("платёж №%d имеет неположительную сумму", p.PaymentNumber)
		}
		sumPrincipal += p.PrincipalAmount
		sumTotal += p.TotalAmount
	}

	// Сумма основного долга по графику должна совпасть с телом кредита.
	if math.Abs(sumPrincipal-principal) > 0.05 {
		t.Errorf("сумма погашения тела = %.2f, ожидалось %.2f", sumPrincipal, principal)
	}
	// Переплата обязана быть положительной при ненулевой ставке.
	if sumTotal <= principal {
		t.Errorf("общая сумма выплат %.2f должна превышать тело кредита", sumTotal)
	}
}

func TestBuildScheduleDueDates(t *testing.T) {
	start := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	schedule := BuildSchedule(1, 60000, 15, 3, start)

	for i, p := range schedule {
		expected := start.AddDate(0, i+1, 0)
		if !p.DueDate.Equal(expected) {
			t.Errorf("платёж №%d: дата %s, ожидалась %s",
				p.PaymentNumber, p.DueDate, expected)
		}
	}
}
