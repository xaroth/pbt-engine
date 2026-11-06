package core

// ActorID identifies an actor within a session: player, enemy, minion type, totem.
type ActorID uint16

// CtxID identifies an interned context within a session. Context 0 is the global context
// (empty mask), used by the character sheet.
type CtxID uint32

// GlobalCtx is the context with no bits set.
const GlobalCtx CtxID = 0

// SourceID is a handle to where modifiers came from (tree node, item mod, scenario field),
// handed out by Session.NewSource. Its meaning lives outside core; core only enables and
// disables it.
type SourceID uint32

// ModID identifies a modifier within a session. The zero ModID is never valid.
type ModID struct {
	n uint32 // slot + 1
}

// Op is how a modifier combines with others on the same stat.
type Op uint8

// Ops. Override replaces the aggregate; Flag sets a Flag stat.
const (
	OpBase Op = iota
	OpInc
	OpMore
	OpOverride
	OpFlag
)

func (o Op) String() string {
	switch o {
	case OpBase:
		return "base"
	case OpInc:
		return "inc"
	case OpMore:
		return "more"
	case OpOverride:
		return "override"
	case OpFlag:
		return "flag"
	}
	return "op?"
}

// Self in a Ref means the actor or context of the node being evaluated.
const Self = -1

// Ref selects a node relative to the node being evaluated. Actor and Ctx are absolute ids, or
// Self.
type Ref struct {
	Actor int32
	Stat  StatID
	Ctx   int64
}

// SelfRef reads stat on the same actor and context as the node being evaluated.
func SelfRef(stat StatID) Ref { return Ref{Actor: Self, Stat: stat, Ctx: Self} }

// Cmp is a comparison used by conditions.
type Cmp uint8

// Comparisons.
const (
	CmpGE Cmp = iota
	CmpGT
	CmpLE
	CmpLT
	CmpEQ
	CmpNE
)

// Cond holds when the referenced value compares true against Threshold.
type Cond struct {
	Ref       Ref
	Cmp       Cmp
	Threshold float64
}

func (c *Cond) holds(v float64) bool {
	switch c.Cmp {
	case CmpGE:
		return v >= c.Threshold
	case CmpGT:
		return v > c.Threshold
	case CmpLE:
		return v <= c.Threshold
	case CmpLT:
		return v < c.Threshold
	case CmpEQ:
		return v == c.Threshold
	default:
		return v != c.Threshold
	}
}

// Scale multiplies a modifier's value by the step count floor(ref / Step). When HasCap is set,
// the step count is at most Cap: "1% per 10 strength, up to 5%" is a modifier of value 1 with
// Step 10 and Cap 5.
type Scale struct {
	Ref    Ref
	Step   float64
	Cap    float64
	HasCap bool
}

// Predicate restricts a modifier to contexts. A context matches when it intersects every mask
// in Req and none of Excl. A zero Predicate matches every context.
type Predicate struct {
	Req  []Mask
	Excl Mask
}

func (p *Predicate) matches(m Mask) bool {
	if m.Intersects(p.Excl) {
		return false
	}
	for i := range p.Req {
		if !m.Intersects(p.Req[i]) {
			return false
		}
	}
	return true
}

// Modifier is one change to a stat of an actor. Conditions must all hold; scales multiply.
type Modifier struct {
	Stat   StatID
	Actor  ActorID
	Op     Op
	Value  float64
	Source SourceID
	Pred   Predicate
	Conds  []Cond
	Scales []Scale
}
