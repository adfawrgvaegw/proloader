package main

import (
	"github.com/Yeah114/Fatalder/control/task"
	"github.com/Yeah114/Fatalder/defines"
)

// convertTasksToDefinesTask 将具体任务类型转换为 defines.Task 接口（泛型版本）
func convertTasksToDefinesTask[T any](tasks ...[]T) []defines.Task {
	var result []defines.Task
	
	for _, taskSlice := range tasks {
		for _, task := range taskSlice {
			// 将任务转换为指针类型，因为 defines.Task 接口是通过指针接收者实现的
			taskPtr := &task
			if taskInterface, ok := any(taskPtr).(defines.Task); ok {
				result = append(result, taskInterface)
			}
		}
	}
	
	return result
}

// convertMixedTasksToDefinesTask 将混合任务类型转换为 defines.Task 接口
func convertMixedTasksToDefinesTask(buildTasks []task.BuildTask, exportTasks []task.ExportTask) []defines.Task {
	var result []defines.Task
	
	// 添加导入任务
	for _, buildTask := range buildTasks {
		result = append(result, &buildTask)
	}
	
	// 添加导出任务
	for _, exportTask := range exportTasks {
		result = append(result, &exportTask)
	}
	
	return result
}
