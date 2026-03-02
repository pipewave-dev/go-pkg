package valkeyadapter

func (vk *valkeyAdapter) Flush() error {
	vk.primClient.Close()
	if vk.repClient != vk.primClient {
		vk.repClient.Close()
	}
	return nil
}
