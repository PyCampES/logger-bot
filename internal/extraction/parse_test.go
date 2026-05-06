package extraction

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Workout
	}{
		{
			name: "es full",
			in:   "Press de banca, 10 repeticiones, 80 kilos, categoría pecho",
			want: Workout{
				Category: "Pecho", Exercise: "Press De Banca",
				Reps: 10, Weight: 80.0, Unit: "kg",
				RawText: "Press de banca, 10 repeticiones, 80 kilos, categoría pecho",
			},
		},
		{
			name: "es minimal",
			in:   "Sentadilla 5 reps 100 kg",
			want: Workout{
				Exercise: "Sentadilla",
				Reps: 5, Weight: 100.0, Unit: "kg",
				RawText: "Sentadilla 5 reps 100 kg",
			},
		},
		{
			name: "en lbs",
			in:   "Shoulder press 8 repetitions 60 lbs",
			want: Workout{
				Exercise: "Shoulder Press",
				Reps: 8, Weight: 60.0, Unit: "lbs",
				RawText: "Shoulder press 8 repetitions 60 lbs",
			},
		},
		{
			name: "missing reps defaults to zero",
			in:   "sentadilla 100 kg",
			want: Workout{
				Exercise: "Sentadilla",
				Reps: 0, Weight: 100.0, Unit: "kg",
				RawText: "sentadilla 100 kg",
			},
		},
		{
			name: "empty input",
			in:   "",
			want: Workout{
				Exercise: "Unknown Exercise",
				Reps: 0, Weight: 0.0, Unit: "kg",
				RawText: "",
			},
		},
		{
			name: "spanish numbers filtered",
			in:   "remo cuatro repeticiones 60 kg",
			want: Workout{
				Exercise: "Remo",
				Reps: 0, Weight: 60.0, Unit: "kg",
				RawText: "remo cuatro repeticiones 60 kg",
			},
		},
		{
			name: "por lado phrase removed",
			in:   "lunge 8 reps por lado 20 kg",
			want: Workout{
				Exercise: "Lunge",
				Reps: 8, Weight: 20.0, Unit: "kg",
				RawText: "lunge 8 reps por lado 20 kg",
			},
		},
		{
			name: "weight before reps with kilos word",
			in:   "press 80 kilos 10 reps",
			want: Workout{
				Exercise: "Press",
				Reps: 10, Weight: 80.0, Unit: "kg",
				RawText: "press 80 kilos 10 reps",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q)\n  got:  %+v\n  want: %+v", tc.in, got, tc.want)
			}
		})
	}
}