package decimal

import (
	"testing"
)

func TestDiv(t *testing.T) {
	tests := []struct {
		name string
		d, o Decimal
		want Decimal // Banker's (To Even)
	}{
		{
			name: "0.5 nanocents -> Div:1, Even:0",
			d:    Epsilon,
			o:    Two,
			want: Decimal(0), // Rounds to Nearest Even (0)
		},
		{
			name: "1.5 nanocents -> Div:2, Even:2",
			d:    Decimal(3),
			o:    Two,
			want: Decimal(2), // Rounds to Nearest Even (2)
		},
		{
			name: "2.5 nanocents -> Div:3, Even:2",
			d:    Decimal(5),
			o:    Two,
			want: Decimal(2), // Rounds to Nearest Even (2)
		},
		{
			name: "-0.5 nanocents -> Div:-1, Even:0",
			d:    -Epsilon,
			o:    Two,
			want: Decimal(0),
		},
		{
			name: "-2.5 nanocents -> Div:-3, Even:-2",
			d:    Decimal(-5),
			o:    Two,
			want: Decimal(-2),
		},
		{
			name: "1.0 / 3.0",
			d:    One,
			o:    Decimal(3 * Scale),
			want: Decimal(33333333),
		},
		{
			name: "2.0 / 3.0",
			d:    Decimal(2 * Scale),
			o:    Decimal(3 * Scale),
			want: Decimal(66666667),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Div(tt.o); got != tt.want {
				t.Errorf("Div() = %v, want %v", got, tt.want)
			}
		})
	}
}
