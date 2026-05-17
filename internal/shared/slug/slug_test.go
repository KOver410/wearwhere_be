package slug

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Áo Thun Trắng", "ao-thun-trang"},
		{"  Café   Sữa  ", "cafe-sua"},
		{"T-Shirt v2.0!", "t-shirt-v2-0"},
		{"---leading---trailing---", "leading-trailing"},
		{"", ""},
		{"!!!", ""},
		{"Số 1 Việt Nam", "so-1-viet-nam"},
		{"Multiple   spaces", "multiple-spaces"},
		{"Đường phố Bùi Thị Xuân", "duong-pho-bui-thi-xuan"},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsValid(t *testing.T) {
	valid := []string{"abc", "abc-def", "a", "a1-b2-c3", "t-shirt"}
	invalid := []string{
		"", "-abc", "abc-", "ab--cd", "Abc", "abc_def", "ab cd", "ab.cd",
	}
	for _, s := range valid {
		if !IsValid(s) {
			t.Errorf("IsValid(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if IsValid(s) {
			t.Errorf("IsValid(%q) = true, want false", s)
		}
	}
}
