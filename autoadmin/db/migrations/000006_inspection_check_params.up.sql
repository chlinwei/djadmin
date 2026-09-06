-- 检查参数化：巡检组声明检查参数（形参），任务绑定时赋值（实参：固定值 / 引用标准变量 / 引用服务宏）。

ALTER TABLE `inspection_group`
  ADD COLUMN `params` json NOT NULL;

ALTER TABLE `inspection_task_group`
  ADD COLUMN `param_values` json NOT NULL;
