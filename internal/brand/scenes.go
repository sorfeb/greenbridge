package brand

import "math"

// bridgeScene is the README's "how it works": commits cross a suspension
// bridge from a private forge (left cliff) to a github.com profile (right).
func bridgeScene() *scene {
	s := newScene(120, 62)
	for y := 46; y < 62; y++ {
		for x := range 120 {
			if y%2 == 0 && (x+y*5)%11 < 4 {
				tok := "l0"
				if y > 53 {
					tok = "l1"
				}
				s.set(float64(x), float64(y), tok)
			}
		}
	}
	rock := func(x, y float64) string { return neutral[1+(int(x)*13+int(y)*7)%3] }
	s.poly([][2]float64{{0, 31}, {15, 30}, {21, 37}, {25, 47}, {27, 62}, {0, 62}}, rock)
	s.poly([][2]float64{{120, 26}, {104, 25}, {99, 34}, {95, 47}, {93, 62}, {120, 62}}, rock)
	s.graph(1, 21, 7, 4, 2, 11, 1, 4)
	s.graph(102, 11, 9, 7, 2, 23, 0, 4)
	for _, xc := range []float64{38, 82} {
		s.cylV(xc-2.5, 1.2, 6, 49, leaf)
		s.cylV(xc+2.5, 1.2, 6, 49, leaf)
		for _, y := range []float64{8, 18, 28} {
			s.rect(xc-2, y, xc+2, y+1, "l1")
		}
		s.shadow(xc, 50, 6, 1.2)
	}
	for x := 38; x <= 82; x++ {
		fx := float64(x)
		y := 7 + 17*(1-(fx-60)/22*((fx-60)/22))
		s.set(fx, y, "l3")
		if x%3 == 0 && x > 39 && x < 81 {
			s.line(fx, y+1, fx, 32, "n3")
		}
	}
	s.line(36, 7, 18, 31, "l3")
	s.line(84, 7, 102, 31, "l3")
	s.cylH(18, 102, 34, 1.6, neutral)
	s.flows = append(s.flows, flow{18, 102, 31, 2, 7, []string{"g1", "g2", "g3", "g4", "g2", "g4", "g3"}})
	return s
}

// Forge brand ramps, dark to light.
var (
	gitlabRamp    = ramp{"#a12c0b", "#e24329", "#fc6d26", "#fca326", "#fdc77a"}
	bitbucketRamp = ramp{"#00296b", "#0052cc", "#2684ff", "#4c9aff", "#b3d4ff"}
	giteaRamp     = ramp{"#3b5e17", "#4e7d1f", "#609926", "#7cb342", "#b5d98c"}
	forgejoOrange = ramp{"#a34100", "#e65c00", "#ff6600", "#ff8c40", "#ffc299"}
	forgejoRed    = ramp{"#7a0000", "#b00000", "#d40000", "#ff3b3b", "#ff9c9c"}
	codebergRamp  = ramp{"#14537e", "#1b6ea8", "#2185d0", "#4ea3e0", "#a8d1f0"}
	gitRamp       = ramp{"#a02b16", "#c93a20", "#f05032", "#f37a63", "#f8b3a6"}
)

// forge is a 24x24 pixel reinterpretation of a git host's logo, lit like the
// rest of the brand art but kept in the host's own colours.
type forge struct {
	slug, name string
	draw       func(s *scene)
}

var forges = []forge{
	{"gitlab", "GitLab", func(s *scene) {
		s.poly([][2]float64{{1, 10}, {4, 1}, {8, 10}}, solid(gitlabRamp[1]))
		s.poly([][2]float64{{23, 10}, {20, 1}, {16, 10}}, solid(gitlabRamp[1]))
		s.poly([][2]float64{{1, 10}, {8, 10}, {12, 23}}, solid(gitlabRamp[3]))
		s.poly([][2]float64{{23, 10}, {16, 10}, {12, 23}}, solid(gitlabRamp[2]))
		s.poly([][2]float64{{8, 10}, {16, 10}, {12, 23}}, shaded(gitlabRamp, 12, 12, 8))
		s.line(4, 2, 6, 8, gitlabRamp[4])
	}},
	{"bitbucket", "Bitbucket", func(s *scene) {
		s.poly([][2]float64{{1, 3}, {23, 3}, {19, 23}, {5, 23}}, shaded(bitbucketRamp, 10, 8, 14))
		s.rect(1, 3, 23, 4, bitbucketRamp[3])
		s.poly([][2]float64{{8, 9}, {16, 9}, {15, 16}, {9, 16}}, solid(bitbucketRamp[0]))
		s.poly([][2]float64{{9, 10}, {15, 10}, {14, 15}, {10, 15}}, solid(bitbucketRamp[4]))
	}},
	{"gitea", "Gitea", func(s *scene) {
		s.ring(19, 13, 2.4, 4.2, giteaRamp)
		s.poly([][2]float64{{2, 7}, {20, 7}, {18, 20}, {4, 20}}, shaded(giteaRamp, 8, 10, 10))
		s.cylH(1, 21, 7, 1, giteaRamp)
		s.shadow(11, 22, 9, 1.2)
		branch(s, [3][2]float64{{9, 10}, {13, 15}, {9, 17}}, .7, 1.3)
	}},
	{"forgejo", "Forgejo", func(s *scene) {
		arc := func(cy float64, r ramp) {
			for a := range 12 {
				t, t2 := math.Pi-float64(a)/12*math.Pi/2, math.Pi-float64(a+1)/12*math.Pi/2
				s.capsule(15+math.Cos(t)*8, cy-math.Sin(t)*8, 15+math.Cos(t2)*8, cy-math.Sin(t2)*8, 1.8, r)
			}
			s.disc(18, cy-8, 2.8, 2.8, r)
		}
		s.capsule(7, 22, 7, 12, 1.8, forgejoOrange)
		arc(20, forgejoRed)
		arc(12, forgejoOrange)
	}},
	{"codeberg", "Codeberg", func(s *scene) {
		s.poly([][2]float64{{12, 2}, {23, 22}, {1, 22}}, shaded(codebergRamp, 9, 9, 14))
		s.poly([][2]float64{{12, 2}, {16, 10}, {12, 13}, {9, 8}}, solid(codebergRamp[4]))
		s.rect(1, 22, 23, 22, codebergRamp[0])
	}},
	{"git", "Any git server", func(s *scene) {
		s.poly([][2]float64{{12, 0}, {24, 12}, {12, 24}, {0, 12}}, shaded(gitRamp, 8, 8, 12))
		branch(s, [3][2]float64{{10, 6}, {15, 13}, {10, 18}}, .8, 1.5)
	}},
	{"self-hosted", "Self-hosted", func(s *scene) {
		for _, y := range []float64{2, 9, 16} {
			s.rect(2, y, 21, y+5, "n2")
			s.rect(2, y, 21, y, "n3")
			s.rect(2, y+5, 21, y+5, "n0")
			s.rect(5, y+2, 13, y+3, "n1")
			s.disc(17, y+2.5, 1.2, 1.2, ramp{"l3"})
			led := "l3"
			if y == 9 {
				led = "l1"
			}
			s.disc(20, y+2.5, 1.2, 1.2, ramp{led})
		}
		s.shadow(12, 23, 10, .8)
	}},
}

// branch draws a git branch glyph in paper colour: a trunk from top to
// bottom node, and a fork from a third of the way down to the side node.
func branch(s *scene, n [3][2]float64, rod, node float64) {
	top, side, bottom := n[0], n[1], n[2]
	paper := ramp{"p"}
	s.capsule(top[0], top[1], bottom[0], bottom[1], rod, paper)
	s.capsule(top[0], top[1]+3, side[0], side[1], rod, paper)
	for _, p := range n {
		s.disc(p[0], p[1], node, node, paper)
	}
}

func forgeScene(f forge) *scene {
	s := newScene(24, 24)
	f.draw(s)
	return s
}
