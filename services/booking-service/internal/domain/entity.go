package domain

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// BookingStatus — типизированные статусы брони.
// Единственный источник правды: используется в usecase, repository, events.
// Порядок переходов: Pending → PaymentPending → Confirmed → Completed
//
//	↘ Cancelled (из любого некомплетного)
type BookingStatus string

const (
	StatusPending        BookingStatus = "pending"
	StatusPaymentPending BookingStatus = "payment_pending"
	StatusConfirmed      BookingStatus = "confirmed"
	StatusCompleted      BookingStatus = "completed"
	StatusCancelled      BookingStatus = "cancelled"
)

// String реализует fmt.Stringer — удобно в логах.
func (s BookingStatus) String() string { return string(s) }

// TimeOfDay — время суток с точностью до минуты (00:00 … 23:59).
// Хранится в БД как PostgreSQL TIME; в Go передаётся как value-object.
// Единственный источник парсинга формата "HH:MM": конструктор ParseTimeOfDay.
// На proto-границе конвертируется через String()/ParseTimeOfDay.
type TimeOfDay struct {
	h, m int // hour [0,23], minute [0,59]
}

// ParseTimeOfDay разбирает строку строго в формате "HH:MM" (ведущие нули обязательны).
// Использует time.Parse("15:04") — отвергает "1:5", "01:5", "1:05" и любые другие
// отклонения от двузначного формата. Диапазоны часов [0,23] и минут [0,59]
// проверяются самим time.Parse через reference time.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return TimeOfDay{}, fmt.Errorf("неверный формат времени %q, ожидается ЧЧ:ММ", s)
	}
	return TimeOfDay{h: t.Hour(), m: t.Minute()}, nil
}

// String возвращает каноническое представление "HH:MM" (без ведущих нулей у часов не будет,
// минуты всегда двузначные). Используется при записи в БД и в proto.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.h, t.m) }

// MinutesSinceStart возвращает количество минут от начала суток.
func (t TimeOfDay) MinutesSinceStart() int { return t.h*60 + t.m }

// After возвращает true если t > other.
func (t TimeOfDay) After(other TimeOfDay) bool {
	return t.MinutesSinceStart() > other.MinutesSinceStart()
}

// MinutesUntil возвращает количество минут от t до other и true если other > t.
// Если other ≤ t — возвращает (0, false), что позволяет вызывающему коду обнаружить
// некорректный порядок без предварительного вызова After.
//
// Идиоматичное использование:
//
//	mins, ok := timeFrom.MinutesUntil(timeTo)
//	if !ok { return pkgerr.InvalidArgument("...") }
func (t TimeOfDay) MinutesUntil(other TimeOfDay) (int, bool) {
	diff := other.MinutesSinceStart() - t.MinutesSinceStart()
	if diff <= 0 {
		return 0, false
	}
	return diff, true
}

// ToTimeIn возвращает time.Time для данного TimeOfDay в заданной локации и дне.
// Удобно для вычисления дедлайна отмены: date + TimeFrom → абсолютный момент визита.
func (t TimeOfDay) ToTimeIn(date time.Time, loc *time.Location) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), t.h, t.m, 0, 0, loc)
}

// Value реализует driver.Valuer — pgx использует при записи в БД (INSERT/UPDATE параметры).
// Возвращает строку "HH:MM", которую PostgreSQL принимает как TIME.
func (t TimeOfDay) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan реализует sql.Scanner — pgx при чтении TIME::TEXT передаёт строку вида "HH:MM:SS"
// или "HH:MM"; мы принимаем оба варианта (секунды игнорируем).
func (t *TimeOfDay) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("TimeOfDay.Scan: ожидается string, получен %T", src)
	}
	// PostgreSQL TIME может вернуть "15:04:05" — обрезаем секунды если есть.
	if len(s) >= 5 {
		s = s[:5]
	}
	parsed, err := ParseTimeOfDay(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

type Booking struct {
	ID      string
	UserID  string
	VenueID string
	// VenueName — намеренная денормализация: имя заведения копируется при создании брони
	// и больше не синхронизируется с venue-service.
	//
	// Обоснование (broken-glass display): пользователь должен видеть корректное название
	// в истории бронирований даже если заведение переименовалось или было удалено.
	// Консистентность с venue-service намеренно не поддерживается — это feature, не bug.
	//
	// При ребрендинге venue: исторические брони сохраняют старое имя (ожидаемое поведение).
	// Если требуется массовое переименование — отдельная one-off миграция по venue_id;
	// см. TECH_DEBT если такая задача появится.
	VenueName string
	ServiceID string
	// PackageServiceIDs — несколько пакетов (из package_service_ids JSON); иначе пусто, см. ServiceID.
	PackageServiceIDs []string
	// HallIDs — выбранные залы (из booking_hall_ids JSON).
	HallIDs    []string
	Date       time.Time
	TimeFrom   TimeOfDay
	TimeTo     TimeOfDay
	Guests     int32
	Comment    string
	Status     BookingStatus
	TotalPrice int64
	PaymentID  string
	PaymentURL string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// StaffNotesCount is a transient, read-only count of internal staff notes,
	// populated only by ListByVenue (CRM registry); zero elsewhere.
	StaffNotesCount int32
}

// IsTerminal возвращает true для статусов из которых нет переходов.
func (b *Booking) IsTerminal() bool {
	return b.Status == StatusCompleted || b.Status == StatusCancelled
}

// CanCancel возвращает true если бронь можно отменить по статусу.
// Не учитывает временной дедлайн — это зона ответственности usecase.
// Инвариант: CanCancel() == !IsTerminal() — намеренно выражено явно.
func (b *Booking) CanCancel() bool {
	return !b.IsTerminal()
}

// CanComplete возвращает true если бронь можно перевести в completed вручную.
func (b *Booking) CanComplete() bool {
	return b.Status == StatusConfirmed && b.PaymentID != ""
}

// BookingStaffNote is an internal venue note on a booking (CRM, not guest-visible).
type BookingStaffNote struct {
	ID           string
	BookingID    string
	VenueID      string
	AuthorUserID string
	Body         string
	CreatedAt    time.Time
}
