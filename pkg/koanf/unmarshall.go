package koanf

func (k *koanfProvider) Unmarshall(output any) error {
	return k.koanf.Unmarshal("", output)
}
