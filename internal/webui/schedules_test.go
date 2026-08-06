package webui

import (
	"testing"

	"github.com/agentic-demo/platform/internal/scheduling"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MapScheduleItem ---

func TestMapScheduleItem(t *testing.T) {
	id := uuid.New()
	s := &scheduling.ReportSchedule{
		ID:       id,
		TenantID: "t_test",
		Type:     scheduling.ScheduleDaily,
		CronExpr: "0 9 * * *",
		Focus:    "Revenue signals",
		Format:   "standard",
		Enabled:  true,
	}

	item := MapScheduleItem(s)
	assert.Equal(t, id.String(), item.ID)
	assert.Equal(t, "daily", item.Type)
	assert.Equal(t, "Daily", item.TypeLabel)
	assert.Equal(t, "info", item.TypeIntent)
	assert.Equal(t, "0 9 * * *", item.CronExpr)
	assert.Equal(t, "Every day at 09:00", item.CronHuman)
	assert.Equal(t, "Revenue signals", item.Focus)
	assert.Equal(t, "standard", item.Format)
	assert.Equal(t, "Standard", item.FormatLabel)
	assert.True(t, item.Enabled)
}

func TestMapScheduleItem_DisabledAndEmptyFormat(t *testing.T) {
	s := &scheduling.ReportSchedule{
		ID:       uuid.New(),
		Type:     scheduling.ScheduleWeekly,
		CronExpr: "0 8 * * 1",
		Format:   "",
		Enabled:  false,
	}

	item := MapScheduleItem(s)
	assert.Equal(t, "Weekly", item.TypeLabel)
	assert.Equal(t, "primary", item.TypeIntent)
	assert.Equal(t, "Every Monday at 08:00", item.CronHuman)
	assert.Equal(t, "Standard", item.FormatLabel)
	assert.False(t, item.Enabled)
}

func TestMapScheduleList(t *testing.T) {
	schedules := []scheduling.ReportSchedule{
		{ID: uuid.New(), Type: scheduling.ScheduleDaily, CronExpr: "0 9 * * *", Enabled: true},
		{ID: uuid.New(), Type: scheduling.ScheduleMonthly, CronExpr: "0 9 1 * *", Enabled: true},
	}

	data := MapScheduleList(schedules)
	require.Len(t, data.Schedules, 2)
	assert.Equal(t, "Daily", data.Schedules[0].TypeLabel)
	assert.Equal(t, "Monthly", data.Schedules[1].TypeLabel)
}

// --- Labels & options ---

func TestScheduleTypeLabel(t *testing.T) {
	assert.Equal(t, "Daily", ScheduleTypeLabel(scheduling.ScheduleDaily))
	assert.Equal(t, "Weekly", ScheduleTypeLabel(scheduling.ScheduleWeekly))
	assert.Equal(t, "Monthly", ScheduleTypeLabel(scheduling.ScheduleMonthly))
	assert.Equal(t, "custom", ScheduleTypeLabel("custom"))
}

func TestScheduleTypeIntent(t *testing.T) {
	assert.Equal(t, "info", ScheduleTypeIntent(scheduling.ScheduleDaily))
	assert.Equal(t, "primary", ScheduleTypeIntent(scheduling.ScheduleWeekly))
	assert.Equal(t, "primary", ScheduleTypeIntent(scheduling.ScheduleMonthly))
	assert.Equal(t, "primary", ScheduleTypeIntent("custom"))
}

func TestScheduleTypeOptions(t *testing.T) {
	opts := ScheduleTypeOptions()
	require.Len(t, opts, 3)
	assert.Equal(t, []ScheduleTypeOption{
		{Value: "daily", Label: "Daily"},
		{Value: "weekly", Label: "Weekly"},
		{Value: "monthly", Label: "Monthly"},
	}, opts)
}

func TestReportFormatOptions(t *testing.T) {
	opts := ReportFormatOptions()
	require.Len(t, opts, 3)
	assert.Equal(t, "standard", opts[0].Value)
	assert.Equal(t, "concise", opts[1].Value)
	assert.Equal(t, "detailed", opts[2].Value)
}

func TestScheduleFormatLabel(t *testing.T) {
	assert.Equal(t, "Standard", ScheduleFormatLabel("standard"))
	assert.Equal(t, "Concise", ScheduleFormatLabel("concise"))
	assert.Equal(t, "Detailed", ScheduleFormatLabel("detailed"))
	assert.Equal(t, "Standard", ScheduleFormatLabel(""))
	assert.Equal(t, "custom", ScheduleFormatLabel("custom"))
}

// --- DescribeCron ---

func TestDescribeCron(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"every minute", "* * * * *", "Every minute"},
		{"daily at 9am", "0 9 * * *", "Every day at 09:00"},
		{"daily at midnight", "0 0 * * *", "Every day at 00:00"},
		{"weekly monday", "0 8 * * 1", "Every Monday at 08:00"},
		{"weekly sunday zero", "0 10 * * 0", "Every Sunday at 10:00"},
		{"weekly sunday seven", "0 10 * * 7", "Every Sunday at 10:00"},
		{"monthly first", "0 9 1 * *", "Monthly on the 1st at 09:00"},
		{"monthly twenty second", "0 9 22 * *", "Monthly on the 22nd at 09:00"},
		{"monthly thirteenth", "0 9 13 * *", "Monthly on the 13th at 09:00"},
		{"custom day and dow", "0 9 1 * 1", "Custom schedule at 09:00"},
		{"too few fields", "0 9 *", "0 9 *"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, DescribeCron(tc.in))
		})
	}
}