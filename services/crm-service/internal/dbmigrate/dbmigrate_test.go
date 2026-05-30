package dbmigrate

import "testing"

func TestMigrationSeq(t *testing.T) {
	tests := []struct {
		name string
		file string
		want int
	}{
		{name: "first", file: "001_venue_staff.up.sql", want: 1},
		{name: "second", file: "002_venue_crm_tasks.up.sql", want: 2},
		{name: "triple_digit", file: "120_thing.up.sql", want: 120},
		{name: "no_underscore", file: "init.up.sql", want: 1 << 30},
		{name: "non_numeric_prefix", file: "abc_thing.up.sql", want: 1 << 30},
		{name: "leading_underscore", file: "_thing.up.sql", want: 1 << 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := migrationSeq(tt.file); got != tt.want {
				t.Fatalf("migrationSeq(%q) = %d, want %d", tt.file, got, tt.want)
			}
		})
	}
}

func TestMigrationSeq_OrdersCorrectly(t *testing.T) {
	if migrationSeq("002_b.up.sql") <= migrationSeq("001_a.up.sql") {
		t.Fatal("expected 002 to sort after 001")
	}
}
