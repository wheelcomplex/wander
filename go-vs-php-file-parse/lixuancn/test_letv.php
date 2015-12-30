<?php
date_default_timezone_set('Asia/Shanghai');

$start = microtime_float();
echo "任务开始:".$start."\n\n";

$fileList = array('test_log2.log');
$bd = array();
$allTotalSize = 0;
foreach($fileList as $file){
    $startTime = microtime_float();
    $handle = fopen($file, 'r');
    $total_size = 0;
    while(!feof($handle)){
        $line = trim(fgets($handle));
        if(strlen($line) < 5){
            continue;
        }
        if(preg_match("/keep_stopp=on/", $line)){
            continue;
        }
        $tmp_arr = explode(' ', $line);
        $size = intval($tmp_arr[8]);
        $total_size += $size;
    }
    echo $file . ':  ' . $total_size."\n";
    $allTotalSize += $total_size;
    $endTime = microtime_float();
    echo "单文件耗时:".($endTime-$startTime)."\n\n";
}
$end = microtime_float();
echo "总耗时:".(($end-$start)*1000)."毫秒\n\n";
echo "总大小:".($allTotalSize)."\n\n";

function microtime_float()
{
    list($usec, $sec) = explode(" ", microtime());
    return ((float)$usec + (float)$sec);
}
?>