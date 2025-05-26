package serhan

type Person struct {
	Name string
	Age  int
}

func (p Person) Hello(age int) *Person {
	return &Person{Name: "Serhan", Age: age}
}

func (p Person) Hello1(age int) Person {
	return Person{Name: "Serhan", Age: age}
}
