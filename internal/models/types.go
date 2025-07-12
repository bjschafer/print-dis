package models

type Printer struct {
	Id         int       `db:"id" json:"id"`
	Name       string    `db:"name" json:"name"`
	Dimensions Dimension `db:"dimensions" json:"dimensions"`
	Url        string    `db:"url" json:"url"`
}

type Dimension struct {
	X int `db:"x" json:"x"`
	Y int `db:"y" json:"y"`
	Z int `db:"z" json:"z"`
}

type Material struct {
	Id   int    `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

type Filament struct {
	Id       int      `db:"id" json:"id"`
	Name     string   `db:"name" json:"name"`
	Material Material `db:"material" json:"material"`
}

type Job struct {
	Id       int       `db:"id" json:"id"`
	Printer  *Printer  `db:"printer" json:"printer"`
	Filament *Filament `db:"filament" json:"filament"`
	Material *Material `db:"material" json:"material"`
}
