package moduledelivery

func (m *moduleDelivery) RunMigration() error {
	return m.repo.RunMigration()
}
