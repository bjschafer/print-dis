package models

type Printer struct {
	Id         int       `db:"id"`
	Name       string    `db:"name"`
	Dimensions Dimension `db:"dimensions"`
	Url        string    `db:"url"`
}

type Dimension struct {
	X int `db:"x"`
	Y int `db:"y"`
	Z int `db:"z"`
}

type Material struct {
	Id   int    `db:"id"`
	Name string `db:"name"`
}

type Filament struct {
	Id       int      `db:"id"`
	Name     string   `db:"name"`
	Material Material `db:"material"`
}

type Job struct {
	Id       int       `db:"id"`
	Printer  *Printer  `db:"printer"`
	Filament *Filament `db:"filament"`
	Material *Material `db:"material"`
}
