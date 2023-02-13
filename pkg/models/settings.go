package models

type Settings struct {
	HopsToSD []TwarpHop
}

type TwarpHop struct {
	Sector int
	Planet int
}
