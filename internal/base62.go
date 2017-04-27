package internal

func Base62(n int32) string {
	const base = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	res := make([]byte, 0, 6)
	for n >= 62 {
		q := n / 62
		r := n % 62
		n = q
		res = append(res, base[r])
	}
	res = append(res, base[n])
	return string(res)
}
