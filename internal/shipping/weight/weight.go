package weight

type Defaults struct {
	WeightG, LengthCM, WidthCM, HeightCM int
}

// Item is one cart line; nil dimension fields fall back to Defaults.
type Item struct {
	Qty      int
	WeightG  *int
	LengthCM *int
	WidthCM  *int
	HeightCM *int
}

// Parcel is the aggregated package sent to the carrier. Goship applies the
// volumetric adjustment itself, so these are actual aggregated values.
type Parcel struct {
	WeightG  int
	LengthCM int
	WidthCM  int
	HeightCM int
}

func or(p *int, def int) int {
	if p != nil && *p > 0 {
		return *p
	}
	return def
}

// Aggregate combines a sub-order's items into one parcel: weight sums by qty,
// length/width take the max footprint, height stacks (Σ qty*height).
func Aggregate(items []Item, d Defaults) Parcel {
	var p Parcel
	for _, it := range items {
		p.WeightG += it.Qty * or(it.WeightG, d.WeightG)
		if l := or(it.LengthCM, d.LengthCM); l > p.LengthCM {
			p.LengthCM = l
		}
		if w := or(it.WidthCM, d.WidthCM); w > p.WidthCM {
			p.WidthCM = w
		}
		p.HeightCM += it.Qty * or(it.HeightCM, d.HeightCM)
	}
	return p
}
