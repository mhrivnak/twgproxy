package models

type Data struct {
	Planets map[int]Planet
	Sectors map[int]Sector
	Status  Status
}

func NewData() *Data {
	return &Data{
		Planets: map[int]Planet{},
		Sectors: map[int]Sector{},
	}
}

type Status struct {
	Creds   int
	Exp     int
	Figs    int
	Sector  int
	Shields int
	Holds   int
}
