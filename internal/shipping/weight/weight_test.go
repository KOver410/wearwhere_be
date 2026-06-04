package weight

import "testing"

func TestAggregate(t *testing.T) {
	d := Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10}
	ip := func(v int) *int { return &v }

	t.Run("explicit fields: weight sums by qty, L/W take max, H stacks", func(t *testing.T) {
		got := Aggregate([]Item{{Qty: 2, WeightG: ip(300), LengthCM: ip(30), WidthCM: ip(25), HeightCM: ip(5)}}, d)
		if got.WeightG != 600 || got.LengthCM != 30 || got.WidthCM != 25 || got.HeightCM != 10 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("missing fields fall back to defaults", func(t *testing.T) {
		got := Aggregate([]Item{{Qty: 3}}, d)
		if got.WeightG != 1500 || got.LengthCM != 20 || got.WidthCM != 15 || got.HeightCM != 30 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("multiple items: max footprint, summed weight + stacked height", func(t *testing.T) {
		got := Aggregate([]Item{
			{Qty: 1, WeightG: ip(200), LengthCM: ip(10), WidthCM: ip(10), HeightCM: ip(4)},
			{Qty: 1, WeightG: ip(800), LengthCM: ip(40), WidthCM: ip(30), HeightCM: ip(6)},
		}, d)
		if got.WeightG != 1000 || got.LengthCM != 40 || got.WidthCM != 30 || got.HeightCM != 10 {
			t.Fatalf("got %+v", got)
		}
	})
}
