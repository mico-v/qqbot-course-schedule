// Command cardpreview renders a sample day card to a JPEG for visual checks.
//
//	go run ./cmd/cardpreview -o /tmp/card.jpg
package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func main() {
	output := flag.String("o", "card-preview.jpg", "output JPEG path")
	rankMode := flag.Bool("rank", false, "render a sample rank board instead of a day card")
	avatarPath := flag.String("avatar", "", "optional avatar image to draw in the card header")
	flag.Parse()

	var avatar image.Image
	if *avatarPath != "" {
		file, err := os.Open(*avatarPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开头像失败:", err)
			os.Exit(1)
		}
		decoded, _, err := image.Decode(file)
		file.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "解析头像失败:", err)
			os.Exit(1)
		}
		avatar = decoded
	}

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

	var image = renderer.DayCard(render.DayCardData{
		Title:         "课程表 · 2026-09-17 周四",
		Footer:        schedule.ScheduleFooter(time.Now(), time.Now()),
		FoldedTitle:   "今天已经没有课的群友",
		Rows:          rows,
		Folded:        folded,
		DurationLabel: "本节持续",
		BotAvatar:     avatar,
	})
	if *rankMode {
		image = renderer.DayCard(render.DayCardData{
			Title:    "群友上课时长榜",
			Subtitle: "2026-09-14..2026-09-20 · 共 4 位成员 · 合计 14小时30分钟",
			Footer:   "重复课程按 RRULE 展开 · 时间以本地时区为准 · 仅展示前 20 名",
			Rows: []schedule.DayRow{
				{UserID: "A1", Name: "小明", StatusKey: "rank1", Status: "#1", Course: "6小时30分钟", TimeText: "8 节 · 5 门课", Duration: "已上 3小时 / 共 6小时30分钟", Progress: 1, CountdownLabel: "时长占比", Countdown: "100%", CourseCount: 8},
				{UserID: "B2", Name: "小红", StatusKey: "rank2", Status: "#2", Course: "5小时", TimeText: "6 节 · 4 门课", Duration: "已上 2小时 / 共 5小时", Progress: 0.77, CountdownLabel: "时长占比", Countdown: "77%", CourseCount: 6},
				{UserID: "C3", Name: "小刚", StatusKey: "rank3", Status: "#3", Course: "3小时", TimeText: "4 节 · 3 门课", Duration: "已上 3小时 / 共 3小时", Progress: 0.46, CountdownLabel: "时长占比", Countdown: "46%", CourseCount: 4},
				{UserID: "D4", Name: "小李", StatusKey: "rank", Status: "#4", Course: "2小时", TimeText: "2 节 · 2 门课", Duration: "已上 0分钟 / 共 2小时", Progress: 0.31, CountdownLabel: "时长占比", Countdown: "31%", CourseCount: 2},
			},
			Legend: []render.LegendItem{
				{Key: "none", Label: "同一时段冲突的课程只计一次"},
				{Key: "none", Label: "全天日程不计入时长"},
			},
		})
	}

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
