package moduledelivery

import "fmt"

func (m *moduleDelivery) RunMigration() error {
	if m.runMigration == nil {
		return fmt.Errorf("migration runner is not configured")
	}
	return m.runMigration()
}
