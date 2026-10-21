package repository

import (
	"ordersystem/model"
	"time"
)

type DatabaseHandler struct {
	// drinks represent all available drinks
	drinks []model.Drink
	// orders serves as order history
	orders []model.Order
}

// todo
func NewDatabaseHandler() *DatabaseHandler {
	// Init the drinks slice with some test data
	drinks := []model.Drink{
		{ID: 1, Name: "Oolong Tea", Price: 3.00, Description: "Traditional semi-oxidized Chinese tea"},
		{ID: 2, Name: "Apple Juice", Price: 2.00, Description: "Freshly made Juice from Apples"},
		{ID: 3, Name: "Sparkling Water", Price: 1.80, Description: "Carbonated Water"},
	}

	// Init orders slice with some test data
	orders := []model.Order{
		{DrinkID: 2, Amount: 3, CreatedAt: time.Now()},
		{DrinkID: 1, Amount: 4, CreatedAt: time.Now()},
		{DrinkID: 3, Amount: 2, CreatedAt: time.Now()},
	}

	return &DatabaseHandler{
		drinks: drinks,
		orders: orders,
	}
}

func (db *DatabaseHandler) GetDrinks() []model.Drink {
	return db.drinks
}

func (db *DatabaseHandler) GetOrders() []model.Order {
	return db.orders
}

// todo
func (db *DatabaseHandler) GetTotalledOrders() map[uint64]uint64 {
	// calculate total orders
	// key = DrinkID, value = Amount of orders
	// totalledOrders map[uint64]uint64
	totalledOrders := make(map[uint64]uint64)
	for _, order := range db.orders {
		totalledOrders[order.DrinkID] += uint64(order.Amount)
	}
	return totalledOrders
}

func (db *DatabaseHandler) AddOrder(order *model.Order) {
	// todo
	// add order to db.orders slice
	db.orders = append(db.orders, *order)
}
