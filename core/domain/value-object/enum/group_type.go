package enum

type GroupType int

const (
	_                GroupType = 0 // Unknown group (should not be used)
	GroupTypePublic  GroupType = 1
	GroupTypePrivate GroupType = 2

	GroupTypeSystem GroupType = 9 // System group
)

func (gt GroupType) IsValid() bool {
	switch gt {
	case GroupTypePublic, GroupTypePrivate, GroupTypeSystem:
		return true
	default:
		return false
	}
}
