package main

import (
	"bufio"
	"fmt"
	"image"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	screenWidth  = 800
	screenHeight = 600
	frameWidth   = 32
	frameHeight  = 32
	tileSize     = 256
)

type Vector2f struct {
	X, Y float64
}

type Character struct {
	bodyTexture        *ebiten.Image
	headTexture        *ebiten.Image
	position           Vector2f
	moveSpeed          float64
	animationSpeed     float64
	frameIndex         int
	direction          int
	timeSinceLastFrame float64
	isMoving           bool
}

func NewCharacter(bodyTexture, headTexture *ebiten.Image, startPos Vector2f) *Character {
	return &Character{
		bodyTexture:    bodyTexture,
		headTexture:    headTexture,
		position:       startPos,
		moveSpeed:      200.0,
		animationSpeed: 0.1,
		frameIndex:     0,
		direction:      0,
		isMoving:       false,
	}
}

func (c *Character) Update(deltaTime float64) {
	c.updateAnimation(deltaTime)
}

func (c *Character) updateAnimation(deltaTime float64) {
	c.timeSinceLastFrame += deltaTime

	if c.isMoving {
		if c.timeSinceLastFrame >= c.animationSpeed {
			c.frameIndex = (c.frameIndex + 1) % 5
			c.timeSinceLastFrame = 0
		}
	} else {
		c.frameIndex = 0
	}
}

func (c *Character) Draw(screen *ebiten.Image, cameraOffset Vector2f) {
	bodyOp := &ebiten.DrawImageOptions{}
	headOp := &ebiten.DrawImageOptions{}

	bodyRow := 0
	if c.isMoving {
		bodyRow = c.frameIndex + 1
	}

	bodyRect := image.Rect(frameWidth*c.direction, frameHeight*bodyRow, frameWidth*(c.direction+1), frameHeight*(bodyRow+1))
	headRect := image.Rect(0, frameHeight*c.direction, frameWidth, frameHeight*(c.direction+1))

	bodyOp.GeoM.Translate(c.position.X-cameraOffset.X, c.position.Y-cameraOffset.Y)
	headOp.GeoM.Translate(c.position.X-cameraOffset.X, c.position.Y-16-cameraOffset.Y)

	screen.DrawImage(c.bodyTexture.SubImage(bodyRect).(*ebiten.Image), bodyOp)
	screen.DrawImage(c.headTexture.SubImage(headRect).(*ebiten.Image), headOp)
}

type Game struct {
	localPlayer  *Character
	otherPlayers map[string]*Character
	conn         net.Conn
	mu           sync.Mutex
	tilesImage   *ebiten.Image
	layers       [][]int
}

func NewGame(conn net.Conn, bodyTexture, headTexture, tilesImage *ebiten.Image, layers [][]int) *Game {
	return &Game{
		localPlayer:  NewCharacter(bodyTexture, headTexture, Vector2f{400, 300}),
		otherPlayers: make(map[string]*Character),
		conn:         conn,
		tilesImage:   tilesImage,
		layers:       layers,
	}
}

func (g *Game) Update() error {
	deltaTime := 1.0 / 120.0
	g.handleInput(deltaTime)
	g.localPlayer.Update(deltaTime)

	g.mu.Lock()
	for _, player := range g.otherPlayers {
		player.Update(deltaTime)
	}
	g.mu.Unlock()

	return nil
}

func (g *Game) handleInput(deltaTime float64) {
	movement := Vector2f{0, 0}
	g.localPlayer.isMoving = false

	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		movement.Y -= g.localPlayer.moveSpeed * deltaTime
		g.localPlayer.direction = 0
		g.localPlayer.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		movement.Y += g.localPlayer.moveSpeed * deltaTime
		g.localPlayer.direction = 2
		g.localPlayer.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		movement.X -= g.localPlayer.moveSpeed * deltaTime
		g.localPlayer.direction = 1
		g.localPlayer.isMoving = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		movement.X += g.localPlayer.moveSpeed * deltaTime
		g.localPlayer.direction = 3
		g.localPlayer.isMoving = true
	}

	g.localPlayer.position.X += movement.X
	g.localPlayer.position.Y += movement.Y

	fmt.Fprintf(g.conn, "%.2f,%.2f,%d,%v\n", g.localPlayer.position.X, g.localPlayer.position.Y, g.localPlayer.direction, g.localPlayer.isMoving)
}

func (g *Game) Draw(screen *ebiten.Image) {
	cameraOffset := Vector2f{
		X: g.localPlayer.position.X - screenWidth/2,
		Y: g.localPlayer.position.Y - screenHeight/2,
	}

	g.drawBackground(screen, cameraOffset)
	g.localPlayer.Draw(screen, cameraOffset)
	g.mu.Lock()
	for _, player := range g.otherPlayers {
		player.Draw(screen, cameraOffset)
	}
	g.mu.Unlock()
}

func (g *Game) drawBackground(screen *ebiten.Image, cameraOffset Vector2f) {
	tileXCount := 400

	const xCount = screenWidth / tileSize
	for _, layer := range g.layers {
		for i, tile := range layer {
			op := &ebiten.DrawImageOptions{}
			x := (i % xCount) * tileSize
			y := (i / xCount) * tileSize
			op.GeoM.Translate(float64(x)-cameraOffset.X, float64(y)-cameraOffset.Y)

			sx := (tile % tileXCount) * tileSize
			sy := (tile / tileXCount) * tileSize
			screen.DrawImage(g.tilesImage.SubImage(image.Rect(sx, sy, sx+tileSize, sy+tileSize)).(*ebiten.Image), op)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) receiveUpdates() {
	reader := bufio.NewReader(g.conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading from server:", err)
			return
		}

		players := strings.Split(strings.TrimSpace(message), ";")
		g.mu.Lock()
		for _, playerData := range players {
			data := strings.Split(playerData, ",")
			if len(data) == 5 {
				id := data[0]
				x, _ := strconv.ParseFloat(data[1], 64)
				y, _ := strconv.ParseFloat(data[2], 64)
				direction, _ := strconv.Atoi(data[3])
				isMoving, _ := strconv.ParseBool(data[4])

				if id != "local" {
					if _, exists := g.otherPlayers[id]; !exists {
						g.otherPlayers[id] = NewCharacter(g.localPlayer.bodyTexture, g.localPlayer.headTexture, Vector2f{x, y})
					}
					g.otherPlayers[id].position = Vector2f{x, y}
					g.otherPlayers[id].direction = direction
					g.otherPlayers[id].isMoving = isMoving
				}
			}
		}
		g.mu.Unlock()
	}
}

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		log.Fatal("Error connecting to server:", err)
	}
	defer conn.Close()

	bodyTexture, _, err := ebitenutil.NewImageFromFile("assets/character.png")
	if err != nil {
		log.Fatal(err)
	}

	headTexture, _, err := ebitenutil.NewImageFromFile("assets/head.png")
	if err != nil {
		log.Fatal(err)
	}
	tilesImage, _, err := ebitenutil.NewImageFromFile("assets/tiles.png")
	if err != nil {
		log.Fatal(err)
	}
	layers := [][]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
	}

	game := NewGame(conn, bodyTexture, headTexture, tilesImage, layers)

	go game.receiveUpdates()

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Multiplayer Game")

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
