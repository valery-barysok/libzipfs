/*
combiner appends a zip file to an executable and further appends a footer
in the last 256 bytes that describes the combination. libzipfs will look
for this footer and use it to determine where the internalized zipfile
filesystem starts.
*/
package libzipfs

import (
	"fmt"
	"io"
	"os"
)

// client take responsibility for closing combFd when done with it; it is the open
// file handled (if err == nil) for reading from the file at combinedPath.
func ReadFooter(combinedPath string) (footerStartOffset int64, ft *Footer, comb *os.File, err error) {

	// read last 256 bytes of combined file and extract the footer
	// cfg.OutputPath is our input now.
	var combi os.FileInfo
	combi, err = os.Stat(combinedPath)
	if err != nil {
		return -1, nil, nil, fmt.Errorf("could not stat path '%s': '%s'", combinedPath, err)
	}
	VPrintf("\n combi = '%#v'\n", combi)

	if combi.Size() < LIBZIPFS_FOOTER_LEN {
		return -1, nil, nil, fmt.Errorf("path to split '%s' smaller (bytes=%d) than "+
			"footer(bytes=%d), cannot be a combiner output file",
			combinedPath, combi.Size(), LIBZIPFS_FOOTER_LEN)
	}

	comb, err = os.Open(combinedPath)
	if err != nil {
		return -1, nil, nil, fmt.Errorf("could not open path '%s': '%s'", combinedPath, err)
	}
	defer func() {
		// don't leak the comb *os.File if returning an error
		if err != nil && comb != nil {
			comb.Close()
		}
	}()

	footerStartOffset, err = comb.Seek(-LIBZIPFS_FOOTER_LEN, 2)
	if err != nil {
		return -1, nil, nil, fmt.Errorf("could not seek to footer position inside file '%s': '%s'",
			combinedPath, err)
	}
	VPrintf("footerStartOffset = %d\n", footerStartOffset)

	by := make([]byte, LIBZIPFS_FOOTER_LEN)
	var n int
	n, err = comb.Read(by)
	if err != io.EOF && err != nil {
		return -1, nil, nil, fmt.Errorf("could not read at footer position inside file '%s': '%s'",
			combinedPath, err)
	}
	if n != LIBZIPFS_FOOTER_LEN {
		return -1, nil, nil, fmt.Errorf("could not read the full footer length from file '%s' "+
			"starting at offset %d: %d == bytes_read_in != LIBZIPFS_FOOTER_LEN == %d",
			combinedPath, footerStartOffset, n, LIBZIPFS_FOOTER_LEN)
	}

	// must return err if foot is bad
	var foot *Footer
	foot, err = ReifyFooterAndDoInexpensiveChecks(by[:], combinedPath, footerStartOffset)
	if err != nil {
		return -1, nil, nil, err
	}
	return footerStartOffset, foot, comb, err
}

func DoSplitOutExeAndZip(cfg *CombinerConfig) (*Footer, error) {

	if cfg.Split != true {
		return nil, fmt.Errorf("DoSplitOutExeAndZip() error: cfg.Split flag "+
			"must be set to true for splitting call. cfg = '%#v'", cfg)
	}

	_, foot, comb, err := ReadFooter(cfg.OutputPath)
	defer comb.Close()

	// create the split out exe and zip files
	exeFd, err := os.Create(cfg.ExecutablePath)
	panicOn(err)
	defer exeFd.Close()

	exeStartOffset, err := comb.Seek(0, 0)
	panicOn(err)
	if exeStartOffset != 0 {
		panic(fmt.Errorf("exeStartOffset was %d but should be 0", exeStartOffset))
	}

	_, err = io.CopyN(exeFd, comb, foot.ExecutableLengthBytes)
	panicOn(err)
	exeFd.Close()

	zipFd, err := os.Create(cfg.ZipfilePath)
	panicOn(err)
	defer zipFd.Close()

	_, err = io.CopyN(zipFd, comb, foot.ZipfileLengthBytes)
	panicOn(err)
	zipFd.Close()

	err = foot.VerifyExeZipChecksums(cfg)

	return foot, err
}

// must return err if foot is bad
func ReifyFooterAndDoInexpensiveChecks(by []byte, combinedPath string, footerStartOffset int64) (*Footer, error) {
	var err error
	var foot Footer
	foot.FromBytes(by[:])

	// NB must use len(MAGIC1) instead of MAGIC_NUM_LEN since len(MAGIC1) is smaller
	_, err = compareByteSlices(foot.MagicFooterNumber1[:len(MAGIC1)], MAGIC1, len(MAGIC1))
	if err != nil {
		return nil, fmt.Errorf("footer magic number1 not found")
	}

	_, err = compareByteSlices(foot.MagicFooterNumber2[:len(MAGIC2)], MAGIC2, len(MAGIC2))
	if err != nil {
		return nil, fmt.Errorf("footer magic number2 not found")
	}

	// check the checksum over the footer itself
	chk := foot.GetFooterChecksum()
	for i := 0; i < 64; i++ {
		if chk[i] != foot.FooterBlake2Checksum[i] {
			return nil, fmt.Errorf("DoSplitOutexeAndZip() error: reified footer from file '%s' does not have the expected checksum, file corrupt or not a combined file?  at i=%d, disk position footerStartOffset=%d, computed footer checksum='%x', versus read-from-disk footer checksum = '%x'", combinedPath, i, footerStartOffset, chk, foot.FooterBlake2Checksum)
		}
	}

	// validate that the component sizes add up
	sumFirstTwo := foot.ZipfileLengthBytes + foot.ExecutableLengthBytes
	if footerStartOffset != sumFirstTwo {
		return nil, fmt.Errorf("DoSplitOutExeAndZip() error: consistency check failed: footerStartOffset(%d) != foot.ZipfileLengthBytes(%d) + foot.ExecutableLengthBytes(%d) == %d", footerStartOffset, foot.ZipfileLengthBytes, foot.ExecutableLengthBytes, sumFirstTwo)
	}

	return &foot, nil
}
