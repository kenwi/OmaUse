package usage

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const zpoolTimeout = 3 * time.Second

// Snapshot is one collection pass.
type Snapshot struct {
	Volumes []Volume
	Err     string
}

type mountEntry struct {
	Source string
	Point  string
	FSType string
	Major  uint32
	Minor  uint32
}

type statFn func(point string) (total, used, free uint64, ok bool)
type labelFn func(source string) string

var skippedFS = map[string]bool{
	"autofs":      true,
	"binfmt_misc": true,
	"bpf":         true,
	"cgroup":      true,
	"cgroup2":     true,
	"configfs":    true,
	"debugfs":     true,
	"devpts":      true,
	"devtmpfs":    true,
	"efivarfs":    true,
	"fusectl":     true,
	"hugetlbfs":   true,
	"mqueue":      true,
	"nfsd":        true,
	"nsfs":        true,
	"overlay":     true,
	"proc":        true,
	"pstore":      true,
	"ramfs":       true,
	"rpc_pipefs":  true,
	"securityfs":  true,
	"squashfs":    true,
	"sysfs":       true,
	"tmpfs":       true,
	"tracefs":     true,
	"zfs":         true,
}

// Collect reads mounted filesystems and ZFS pools.
// Missing or unreadable pools are ignored so hosts without ZFS stay quiet.
func Collect() Snapshot {
	data, err := os.ReadFile("/proc/self/mountinfo")
	var mounts []mountEntry
	fsErr := ""
	if err != nil {
		fsErr = "filesystems could not be read"
	} else {
		mounts = parseMountInfo(bytes.NewReader(data))
	}
	labels := loadLabels()
	vols := volumesFromMounts(mounts, statVolume, func(source string) string {
		return labelFor(source, labels)
	})
	pools, poolErr := readPools(mounts)
	if poolErr != "" {
		return Snapshot{Volumes: vols, Err: fsErr}
	}
	return Snapshot{Volumes: append(vols, pools...), Err: fsErr}
}

func skipFS(fstype string) bool {
	if skippedFS[fstype] {
		return true
	}
	return strings.HasPrefix(fstype, "fuse.portal") || strings.HasPrefix(fstype, "fuse.gvfs")
}

func parseMountInfo(r io.Reader) []mountEntry {
	sc := bufio.NewScanner(r)
	var out []mountEntry
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		dash := -1
		for i, f := range fields {
			if f == "-" {
				dash = i
				break
			}
		}
		if dash < 6 || dash+2 >= len(fields) {
			continue
		}
		major, minor, ok := parseDev(fields[2])
		if !ok {
			continue
		}
		out = append(out, mountEntry{
			Source: unescapeMount(fields[dash+2]),
			Point:  unescapeMount(fields[4]),
			FSType: unescapeMount(fields[dash+1]),
			Major:  major,
			Minor:  minor,
		})
	}
	return out
}

func parseDev(s string) (uint32, uint32, bool) {
	maj, min, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, false
	}
	a, err1 := strconv.ParseUint(maj, 10, 32)
	b, err2 := strconv.ParseUint(min, 10, 32)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return uint32(a), uint32(b), true
}

func unescapeMount(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			v, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
			if err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (m mountEntry) devKey() string {
	return strconv.FormatUint(uint64(m.Major), 10) + ":" + strconv.FormatUint(uint64(m.Minor), 10)
}

// chooseMounts drops virtual filesystems and keeps one row per block device.
// Btrfs subvolumes share a device, so the shortest mount wins and "/" wins ties of purpose.
func chooseMounts(mounts []mountEntry) []mountEntry {
	order := make([]string, 0)
	best := map[string]mountEntry{}
	for _, m := range mounts {
		if skipFS(m.FSType) || m.Point == "" {
			continue
		}
		key := m.devKey()
		prev, ok := best[key]
		if !ok {
			order = append(order, key)
			best[key] = m
			continue
		}
		if preferPoint(m.Point, prev.Point) {
			best[key] = m
		}
	}
	out := make([]mountEntry, 0, len(order))
	for _, key := range order {
		out = append(out, best[key])
	}
	return out
}

func preferPoint(a, b string) bool {
	if a == "/" {
		return true
	}
	if b == "/" {
		return false
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func volumesFromMounts(mounts []mountEntry, stats statFn, label labelFn) []Volume {
	chosen := chooseMounts(mounts)
	out := make([]Volume, 0, len(chosen))
	for _, m := range chosen {
		lab := ""
		if label != nil {
			lab = label(m.Source)
		}
		var total, used, free uint64
		ok := false
		if stats != nil {
			total, used, free, ok = stats(m.Point)
		}
		out = append(out, makeFSVolume(m, lab, total, used, free, ok))
	}
	return out
}

func makeFSVolume(m mountEntry, label string, total, used, free uint64, ok bool) Volume {
	has := ok && total > 0
	pct := 0.0
	if has {
		pct = usePercent(used, free)
	}
	return Volume{
		Name:    volumeName(label, m.Point, m.Source),
		Kind:    m.FSType,
		Where:   m.Point,
		Total:   total,
		Used:    used,
		Free:    free,
		UsePct:  pct,
		HasSize: has,
		Health:  "-",
	}
}

// usePercent matches df: used / (used + available), rounded up.
func usePercent(used, free uint64) float64 {
	denom := used + free
	if denom == 0 || used == 0 {
		return 0
	}
	if used > math.MaxUint64/100 {
		pct := math.Ceil(float64(used) / float64(denom) * 100)
		if pct > 100 {
			return 100
		}
		return pct
	}
	scaled := used * 100
	pct := scaled / denom
	if scaled%denom != 0 {
		pct++
	}
	if pct > 100 {
		return 100
	}
	return float64(pct)
}

func volumeName(label, point, source string) string {
	if label != "" {
		return label
	}
	if point != "" && point != "/" {
		base := baseName(point)
		if base != "" {
			return base
		}
	}
	base := baseName(source)
	if base == "" {
		if source != "" {
			return source
		}
		return point
	}
	return base
}

func baseName(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return ""
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func statVolume(point string) (total, used, free uint64, ok bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(point, &st); err != nil {
		return 0, 0, 0, false
	}
	if st.Bsize <= 0 || st.Blocks == 0 {
		return 0, 0, 0, false
	}
	bsize := uint64(st.Bsize)
	total = st.Blocks * bsize
	free = st.Bavail * bsize
	if st.Blocks >= st.Bfree {
		used = (st.Blocks - st.Bfree) * bsize
	}
	return total, used, free, true
}

func loadLabels() map[uint64]string {
	out := map[uint64]string{}
	entries, err := os.ReadDir("/dev/disk/by-label")
	if err != nil {
		return out
	}
	for _, e := range entries {
		id, ok := deviceRdev("/dev/disk/by-label/" + e.Name())
		if !ok {
			continue
		}
		if _, exists := out[id]; !exists {
			out[id] = unescapeUdev(e.Name())
		}
	}
	return out
}

func labelFor(source string, labels map[uint64]string) string {
	if len(labels) == 0 {
		return ""
	}
	id, ok := deviceRdev(source)
	if !ok {
		return ""
	}
	return labels[id]
}

func deviceRdev(path string) (uint64, bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, false
	}
	kind := st.Mode & unix.S_IFMT
	if kind != unix.S_IFBLK && kind != unix.S_IFCHR {
		return 0, false
	}
	return uint64(st.Rdev), true
}

func unescapeUdev(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' {
			v, err := strconv.ParseUint(s[i+2:i+4], 16, 8)
			if err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

type parsedPool struct {
	vol     Volume
	altroot string
}

func readPools(mounts []mountEntry) ([]Volume, string) {
	ctx, cancel := context.WithTimeout(context.Background(), zpoolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "zpool", "list", "-Hp", "-o", "name,size,alloc,free,cap,health,altroot")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, poolReadError(err, &stderr, &stdout)
	}
	pools, err := parseZpool(&stdout)
	if err != nil {
		return nil, "pools could not be read"
	}
	return placePools(pools, mounts), ""
}

func poolReadError(err error, stderr, stdout *bytes.Buffer) string {
	const prefix = "pools could not be read"
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(stdout.String())
	}
	if detail == "" {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail = strings.TrimSpace(string(exitErr.Stderr))
		}
	}
	if detail == "" {
		return prefix
	}
	if i := strings.IndexByte(detail, '\n'); i >= 0 {
		detail = detail[:i]
	}
	return prefix + ": " + detail
}

func parseZpool(r io.Reader) ([]parsedPool, error) {
	sc := bufio.NewScanner(r)
	var out []parsedPool
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) == 1 {
			fields = strings.Fields(line)
		}
		if len(fields) < 6 {
			return nil, errors.New("bad zpool line")
		}
		size, err1 := strconv.ParseUint(fields[1], 10, 64)
		alloc, err2 := strconv.ParseUint(fields[2], 10, 64)
		free, err3 := strconv.ParseUint(fields[3], 10, 64)
		capPct, err4 := strconv.ParseFloat(fields[4], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			return nil, errors.New("bad zpool numbers")
		}
		health := fields[5]
		if health == "" {
			health = "-"
		}
		alt := "-"
		if len(fields) >= 7 && fields[6] != "" {
			alt = fields[6]
		}
		name := fields[0]
		if name == "" {
			return nil, errors.New("bad zpool name")
		}
		out = append(out, parsedPool{
			vol: Volume{
				Name:    name,
				Kind:    "zfs pool",
				Where:   "-",
				Total:   size,
				Used:    alloc,
				Free:    free,
				UsePct:  capPct,
				HasSize: size > 0,
				Health:  health,
			},
			altroot: alt,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func placePools(pools []parsedPool, mounts []mountEntry) []Volume {
	out := make([]Volume, 0, len(pools))
	for _, p := range pools {
		p.vol.Where = poolWhere(p.vol.Name, p.altroot, mounts)
		out = append(out, p.vol)
	}
	return out
}

func poolWhere(name, altroot string, mounts []mountEntry) string {
	best := ""
	for _, m := range mounts {
		if m.FSType != "zfs" || m.Source != name {
			continue
		}
		if best == "" || preferPoint(m.Point, best) {
			best = m.Point
		}
	}
	if best != "" {
		return best
	}
	if altroot != "" && altroot != "-" {
		return altroot
	}
	return "-"
}
