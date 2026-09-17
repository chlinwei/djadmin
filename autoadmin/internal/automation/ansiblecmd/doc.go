// Package ansiblecmd 统一 autoadmin 执行 ansible-playbook 的方式。
//
// 两个构建变体：
//   - 默认（不带 tag）：用宿主 PATH 上的 `ansible-playbook`，部署机需自行安装
//     Python + ansible-core（当前生产口径）。
//   - `-tags embedansible`：把 CPython + ansible-core 打进二进制，自包含，部署机
//     不再需要宿主 Python/ansible-core。ansible-core 版本钉在 requirements.txt
//     （当前 2.16.x：目标机 Python 3.6 兼容；2.17+ 起要求目标机 ≥3.7）。
//
// 生成嵌入数据（仅 embed 变体需要，requirements.txt 变更后重跑）：
//
//	make ansible-embed
//
// 注意：ansible-core 是 GPL-3.0-or-later，随二进制分发需评估许可证义务。
package ansiblecmd

//go:generate go run ./_generate
