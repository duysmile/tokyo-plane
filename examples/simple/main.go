// go run main.go -server ws://{host}/socket -key={unique id} -name={display name}
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	tokyo "github.com/ledongthuc/tokyo_go_sdk"
)

var server = flag.String("server", "", "server host")
var userKey = flag.String("key", "", "user's key")
var userName = flag.String("name", "", "user's name")

var MAX_SAFE = 120
var MAX_SIZE = 8000.0

func main() {
	flag.Parse()
	validateParams()
	log.Printf("Start server: %s, key: %s, name: %s", *server, *userKey, *userName)

	var id int64 = -1
	var angle float64
	var shouldFireBefore bool
	var shouldFireAfter bool

	client := tokyo.NewClient(*server, *userKey, *userName)
	client.RegisterStateEventHandler(func(e tokyo.StateEvent) {
		if id != -1 && len(e.Data.Players) > 0 {
			myPlayer, otherPlayers, otherBullets := findMyPlayer(
				e.Data.Players,
				e.Data.Bullets,
				id,
			)
			if myPlayer == nil {
				log.Fatal("END")
			}

			// angle = calculateAngle(*myPlayer, otherPlayer)
			angle, shouldFireBefore, shouldFireAfter = calculateAngle(*myPlayer, otherPlayers, otherBullets)
			// distance = calculateDistance(*myPlayer, otherPlayers)
		}
	})
	client.RegisterCurrentUserIDEventHandler(func(e tokyo.CurrentUserIDEvent) {
		log.Printf("User ID Event: %+v", e)
		id = e.Data
	})
	client.RegisterTeamNamesEventHandler(func(e tokyo.TeamNamesEvent) {
		// log.Printf("Team names: %+v", e)
	})

	setupCloseHandler(*client)

	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		defer ticker.Stop()
		for {
			_ = <-ticker.C
			if !client.ConnReady {
				continue
			}

			if shouldFireBefore {
				client.Fire()
			}
			client.Rotate(angle + rand.Float64()*0.001)
			if shouldFireAfter {
				client.Fire()
			}
			client.Throttle(1)
			// client.Fire()
		}
	}()
	log.Fatal(client.Listen())
}

func setupCloseHandler(client tokyo.Client) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		client.Close()
		os.Exit(0)
	}()
}

func validateParams() {
	if server == nil {
		panic("miss server flag")
	}
	if userKey == nil {
		panic("miss key flag")
	}
	if userName == nil {
		panic("miss name flag")
	}
}

func findMyPlayer(
	players []tokyo.Player,
	bullets []tokyo.Bullet,
	id int64,
) (*tokyo.Player, []tokyo.Player, []tokyo.Bullet) {
	otherBullets := make([]tokyo.Bullet, len(bullets))
	var currentPlayer tokyo.Player
	var index int

	for i, player := range players {
		if player.ID == id {
			currentPlayer = player
			index = i
			break
		}
	}

	otherPlayers := append(players[:index], players[index+1:]...)

	for index := range bullets {
		if bullets[index].PlayerID != id {
			otherBullets = append(otherBullets, bullets[index])
		}
	}

	return &currentPlayer, otherPlayers, otherBullets
}

type Point struct {
	X float64
	Y float64
}

func calculateAngleFrom2Point(a, b Point) float64 {
	return math.Atan2(b.Y-a.Y, b.X-a.X)
}

func calculateDistanceFrom2Point(a, b Point) float64 {
	return math.Sqrt(math.Pow(b.Y-a.Y, 2) + math.Pow(b.X-a.X, 2))
}

func calculateAngle(
	player tokyo.Player,
	others []tokyo.Player,
	bullets []tokyo.Bullet,
) (float64, bool, bool) {
	minDistance := MAX_SIZE
	var nearestPlayer tokyo.Player

	if player.X == 0 || player.Y == 0 {
		return math.Pi - player.Angle, false, false
	}

	for _, otherPlayer := range others {
		distance := calculateDistanceFrom2Point(
			Point{player.X, player.Y},
			Point{otherPlayer.X, otherPlayer.Y},
		)

		// log.Println("Distance: ", distance)

		if minDistance >= distance {
			minDistance = distance
			nearestPlayer = otherPlayer
		}
	}

	for _, bullet := range bullets {
		distance := calculateDistanceFrom2Point(
			Point{player.X, player.Y},
			Point{bullet.X, bullet.Y},
		)

		angle := calculateAngleFrom2Point(
			Point{bullet.X, bullet.Y},
			Point{player.X, player.Y},
		)

		if distance <= float64(MAX_SAFE+100) {
			if player.Angle+angle == math.Pi {
				return math.Pi / 2, false, false
			}
			if player.Angle == bullet.Angle ||
				angle == bullet.Angle {
				log.Println("Ne")
				return math.Pi / 4, false, false
			}
		}
	}

	anglePlayer := calculateAngleFrom2Point(
		Point{player.X, player.Y},
		Point{nearestPlayer.X, nearestPlayer.Y},
	)

	if minDistance <= float64(MAX_SAFE) {
		if checkIfEnemyBehind(player, nearestPlayer) {
			return math.Pi / 2, false, false
		}

		if checkIfEnemyFront(player, nearestPlayer, anglePlayer) {
			return anglePlayer, false, true
		}

		return player.Angle - math.Pi/4, true, false
	}

	// client.Fire()
	return anglePlayer, false, minDistance <= float64(MAX_SAFE)+200
}

func checkIfEnemyBehind(a, b tokyo.Player) bool {
	isBehindI := a.Angle >= 0 &&
		a.Angle <= math.Pi/2 &&
		(a.X > b.X || a.Y > b.Y)

	isBehindII := a.Angle > math.Pi/2 &&
		a.Angle <= math.Pi &&
		(a.X < b.X || a.Y > b.Y)

	isBehindIII := a.Angle > math.Pi &&
		a.Angle <= math.Pi*3/2 &&
		(a.X < b.X || a.Y < b.Y)

	isBehindIV := a.Angle > math.Pi*3/2 &&
		a.Angle < 2*math.Pi &&
		(a.X > b.X || a.Y < b.Y)

	return a.Angle == b.Angle && (isBehindI ||
		isBehindII ||
		isBehindIII ||
		isBehindIV)
}

func checkIfEnemyFront(a, b tokyo.Player, angle float64) bool {
	isFrontI := a.Angle >= 0 &&
		a.Angle <= math.Pi/2 &&
		(a.X < b.X || a.Y < b.Y)

	isFrontII := a.Angle > math.Pi/2 &&
		a.Angle <= math.Pi &&
		(a.X > b.X || a.Y < b.Y)

	isFrontIII := a.Angle > math.Pi &&
		a.Angle <= math.Pi*3/2 &&
		(a.X > b.X || a.Y > b.Y)

	isFrontIV := a.Angle > math.Pi*3/2 &&
		a.Angle < 2*math.Pi &&
		(a.X < b.X || a.Y > b.Y)

	// isOnALine := angle == a.Angle || angle == b.Angle

	return (a.Angle == b.Angle || a.Angle+b.Angle == math.Pi) &&
		(isFrontI ||
			isFrontII ||
			isFrontIII ||
			isFrontIV)
}
