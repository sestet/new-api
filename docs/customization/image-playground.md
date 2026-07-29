# GPT Image Playground 集成

## 接入方式

GPT Image Playground 作为登录后的独立菜单和路由接入现有前端，不要求用户再次填写 API URL 或 API Key。

- 页面从当前用户已有的 API Token 中选择一个可用 Token。
- 选择 Token 后，通过受保护的 Token 明文接口读取一次完整 Key，并只缓存在当前页面运行内存中。
- 模型列表通过同源 `GET /v1/models` 获取，因此不同 Token 会按各自权限和分组返回模型。
- 生图和编辑请求使用 `${window.location.origin}/v1`，生产环境直接访问同域后端；本地 `5173` 由 Rsbuild 将 `/v1` 转发到后端。
- 默认优先选择名称包含 `image` 或 `dall-e` 的模型，没有匹配时使用第一项，最终回退到 `gpt-image-2`。

## 数据存储

后端不额外保存 Playground 图片和任务历史。浏览器使用按用户 ID 隔离的 IndexedDB 保存任务、图片、缩略图和对话记录，数据库名格式为：

```text
tlabcode-image-playground:user:<user_id>
```

最近选择的 Token ID 按用户保存在 `localStorage`，完整 API Key 不写入 `localStorage` 或 IndexedDB。清理浏览器站点数据会同时清除本地图片历史。

## 使用边界

- 页面卸载会中断仍在进行的浏览器请求，因此页面明确提示生图期间留在当前页面。
- 只有状态正常、未过期且仍有额度或不限额的 Token 可选择。
- Token 可见模型不同是后端权限、分组或模型范围的结果，不由 Playground 自行合并。
- 图片请求继续走统一 relay、渠道调度、扣费和上游精准对账链路。

原始界面基于 [GPT Image Playground](https://github.com/CookSleep/gpt_image_playground) 的 MIT 许可代码进行集成，许可文件位于 `web/src/features/gpt-image-playground/external/LICENSE.gpt-image-playground`。
