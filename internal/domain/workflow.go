package domain

import "strings"

type OrderState struct {
	Status                string
	AssigneeID            *int64
	ReassignmentRequested bool
}
type Actor struct {
	ID   int64
	Role string
}
type OrderChange struct {
	Status     string `json:"status"`
	Action     string `json:"action"`
	AssigneeID *int64 `json:"assignee_id"`
	Note       string `json:"note"`
}
type WorkflowError struct{ Code, Message string }

func (e *WorkflowError) Error() string { return e.Message }
func fail(code, message string) error  { return &WorkflowError{code, message} }

// PlanOrderChange owns role and state policy. Database lookups and audit writes
// happen in the caller's transaction, after this policy has accepted the action.
func PlanOrderChange(current OrderState, actor Actor, in OrderChange) (string, error) {
	manager := actor.Role == "admin" || actor.Role == "operator"
	owner := actor.Role == "technician" && current.AssigneeID != nil && *current.AssigneeID == actor.ID
	active := current.Status == "assigned" || current.Status == "in_progress" || current.Status == "on_hold"
	if !manager && !owner {
		return "", fail("forbidden", "Yalnızca size atanmış iş üzerinde işlem yapabilirsiniz")
	}
	if len(in.Note) > 10000 {
		return "", fail("invalid", "Not çok uzun")
	}
	note := strings.TrimSpace(in.Note) != ""
	if in.Action != "" && in.Status != "" {
		return "", fail("invalid", "İşlem ve durum aynı anda gönderilemez")
	}
	switch in.Action {
	case "request_reassignment":
		if !owner {
			return "", fail("forbidden", "Devir talebini yalnızca sorumlu teknisyen oluşturabilir")
		}
		if !active || current.ReassignmentRequested {
			return "", fail("conflict", "Bu iş için yeni devir talebi oluşturulamaz")
		}
		if !note || in.AssigneeID != nil {
			return "", fail("invalid", "Devir talebi için gerekçe gerekli; personel seçilemez")
		}
		return current.Status, nil
	case "reassign":
		if !manager {
			return "", fail("forbidden", "Yeniden atama yönetici veya operatör tarafından yapılır")
		}
		if !active {
			return "", fail("conflict", "Yalnızca aktif işler yeniden atanabilir")
		}
		if !note || in.AssigneeID == nil || *in.AssigneeID <= 0 {
			return "", fail("invalid", "Yeni teknisyen ve atama gerekçesi gerekli")
		}
		if current.AssigneeID != nil && *current.AssigneeID == *in.AssigneeID {
			return "", fail("conflict", "Farklı bir teknisyen seçin")
		}
		return "assigned", nil
	case "return":
		if !manager {
			return "", fail("forbidden", "Çözümü yalnızca yönetici veya operatör geri gönderebilir")
		}
		if current.Status != "resolved" {
			return "", fail("conflict", "Yalnızca çözülmüş işler geri gönderilebilir")
		}
		if !note || in.AssigneeID != nil {
			return "", fail("invalid", "Geri gönderme gerekçesi gerekli; personel değiştirilemez")
		}
		return "assigned", nil
	case "":
	default:
		return "", fail("invalid", "Geçersiz işlem")
	}
	if !CanTransition(current.Status, in.Status) {
		return "", fail("conflict", "Geçersiz durum geçişi")
	}
	if in.Status == "assigned" || in.Status == "closed" {
		if !manager {
			return "", fail("forbidden", "Atama ve kapatma yönetici veya operatör tarafından yapılır")
		}
	} else if !owner {
		return "", fail("forbidden", "İşi başlatma, bekletme ve çözme sorumlu teknisyene aittir")
	}
	if in.Status == "assigned" {
		if in.AssigneeID == nil || *in.AssigneeID <= 0 {
			return "", fail("invalid", "Teknisyen seçilmeli")
		}
	} else if in.AssigneeID != nil {
		return "", fail("invalid", "Bu işlemde personel değiştirilemez")
	}
	if (in.Status == "on_hold" || in.Status == "resolved") && !note {
		return "", fail("invalid", "Bekleme veya çözüm gerekçesi gerekli")
	}
	return in.Status, nil
}
