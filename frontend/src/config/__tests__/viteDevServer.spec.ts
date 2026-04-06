import { readFileSync } from 'fs'
import { resolve } from 'path'

describe('vite dev server defaults', () => {
  it('uses a fixed frontend port for local development', () => {
    const viteConfig = readFileSync(resolve(__dirname, '../../../vite.config.ts'), 'utf-8')

    expect(viteConfig).toContain("const devPort = Number(env.VITE_DEV_PORT || 3001)")
    expect(viteConfig).toContain("host: '0.0.0.0'")
    expect(viteConfig).toContain('strictPort: true')
  })
})
