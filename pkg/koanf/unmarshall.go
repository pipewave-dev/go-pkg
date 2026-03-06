package koanf

func (k *koanfProvider) Unmarshall(output any) {
	_ = k.koanf.Unmarshal("", output)
}
