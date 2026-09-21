// Command cardpreview renders a sample day card to a JPEG for visual checks.
//
//	go run ./cmd/cardpreview -o /tmp/card.jpg
package main

import (
	"flag"
	"fmt"
	"image/jpeg"
	"os"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func main() {
	output := flag.String("o", "card-preview.jpg", "output JPEG path")
	flag.Parse()

	renderer, err := render.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载字体失败:", err)
		os.Exit(1)
	}

	rows := []schedule.DayRow{
		{
			UserID: "A1B2C3D4E5F6", Name: "小明", StatusKey: "active", Status: "正在上课",
			Course: "高等数学", Location: "教一101", TimeText: "09:00 - 10:30",
			Duration: "1小时30分钟", CountdownLabel: "距下课", Countdown: "45分钟",
			Progress: 0.5, CourseCount: 2,
		},
		{
			UserID: "B2C3D4E5F6A1", Name: "小红", StatusKey: "upcoming", Status: "下一节即将上课",
			Course: "大学英语", Location: "外语楼203", TimeText: "11:00 - 12:00",
			Duration: "1小时", CountdownLabel: "距上课", Countdown: "1小时30分钟", CourseCount: 1,
		},
		{
			UserID: "C3D4E5F6A1B2", Name: "小刚", StatusKey: "upcoming", Status: "下一节即将上课",
			Course: "数据结构", TimeText: "14:00 - 15:30", Duration: "1小时30分钟",
			CountdownLabel: "距上课", Countdown: "4小时30分钟", CourseCount: 3,
		},
	}
	folded := []schedule.DayRow{
		{UserID: "D4E5F6A1B2C3", Name: "小李", StatusKey: "none", Status: "今日无课"},
		{UserID: "E5F6A1B2C3D4", Name: "小王", StatusKey: "finished", Status: "今日课程已结束"},
		{UserID: "F6A1B2C3D4E5", Name: "小张", StatusKey: "holiday", Status: "今日休假"},
	}

	image := renderer.DayCard(render.DayCardData{
		Title:       "课程表 · 2026-09-17 周四",
		Footer:      schedule.ScheduleFooter(time.Now(), time.Now()),
		FoldedTitle: "今天已经没有课的群友",
		Rows:        rows,
		Folded:      folded,
	})

	file, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建文件失败:", err)
		os.Exit(1)
	}
	defer file.Close()
	if err := jpeg.Encode(file, image, &jpeg.Options{Quality: 90}); err != nil {
		fmt.Fprintln(os.Stderr, "编码失败:", err)
		os.Exit(1)
	}
	fmt.Println("已生成", *output)
}
