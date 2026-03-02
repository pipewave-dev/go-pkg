package koanf

func (k *koanfProvider) Unmarshall(output any) {
	k.koanf.Unmarshal("", output)
}
