<?php
date_default_timezone_set('Asia/Shanghai');

$file = @$argv[2];

if (strlen($file) == 0) {
	printf("USAGE: php parser.php -f <input text file | 100m.lines.testdata.txt>\n");
	exit(1);
}

$filemb = filesize($file) / 1024 / 1024;

$handle = fopen($file, 'r');
if ($handle == FALSE) {
    printf("Error: open file %s for read failed: %s\n", $file);
    exit(1);
}
$totalparsesize = 0;
$totalline = 0;
$totalbytes = 0;

printf("php 7.0, parsing file %s(%d MB) ...\n", $file, $filemb);

$starttime = explode(' ',microtime());
while(!feof($handle)){
    $line = trim(fgets($handle));
    $totalline++;
    $totalbytes += strlen($line);
    if(strlen($line) < 5){
        continue;
    }
    $tmp_arr = explode(' ', $line);
    $size = intval($tmp_arr[8]);
    $totalparsesize += $size;
}
$endtime = explode(' ',microtime());
$thistime = $endtime[0]+$endtime[1]-($starttime[0]+$starttime[1]);
$thistime = round($thistime,3);
$mbsize = $totalbytes / 1024 / 1024;
if ($thistime <= 0) {
    $thistime = 1;
}
$bw = $mbsize / $thistime;
printf("php, parsed file %s in %s, %d lines, %d MB, speed %d MB/s\n", $file, $thistime, $totalline, $mbsize, $bw);
?>
