import * as THREE from "three";
import { EffectComposer } from "three/examples/jsm/postprocessing/EffectComposer.js";
import { RenderPass } from "three/examples/jsm/postprocessing/RenderPass.js";
import { UnrealBloomPass } from "three/examples/jsm/postprocessing/UnrealBloomPass.js";
import { ShaderPass } from "three/examples/jsm/postprocessing/ShaderPass.js";

/**
 * Split-RGB chromatic aberration, dynamic film grain, and velocity-tightening vignette
 * as specified in the original Gladeye engine recreation.
 */
const SplitRGBNoiseShader = {
  name: "SplitRGBNoiseShader",
  uniforms: {
    tDiffuse: { value: null },
    uTime: { value: 0 },
    uVelocity: { value: 0 },
    uResolution: { value: new THREE.Vector2(1600, 900) },
  },
  vertexShader: `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: `
    uniform sampler2D tDiffuse;
    uniform float uTime;
    uniform float uVelocity;
    uniform vec2 uResolution;
    varying vec2 vUv;

    // Pseudo-random film grain generator
    float hash(vec2 p) {
      vec3 p3 = fract(vec3(p.xyx) * 0.1031);
      p3 += dot(p3, p3.yzx + 33.33);
      return fract((p3.x + p3.y) * p3.z);
    }

    void main() {
      vec2 uv = vUv;
      vec2 center = uv - 0.5;
      float dist = length(center);

      // Split-RGB chromatic dispersion: stronger at perimeter and expands during movement
      float aberration = (0.0035 + uVelocity * 0.012) * dist;
      vec2 rOffset = center * aberration;
      vec2 bOffset = -center * aberration;

      float r = texture2D(tDiffuse, uv + rOffset).r;
      float g = texture2D(tDiffuse, uv).g;
      float b = texture2D(tDiffuse, uv + bOffset).b;

      vec3 col = vec3(r, g, b);

      // Organic film grain noise
      vec2 grainCoord = uv * uResolution * 0.75 + vec2(sin(uTime * 37.0), cos(uTime * 43.0)) * 60.0;
      float grain = (hash(grainCoord) - 0.5) * 0.052;
      col += vec3(grain);

      // Vignette that tightens as you move faster (original engine requirement)
      float vigRadius = 0.88 - clamp(uVelocity * 2.4, 0.0, 0.38);
      float vigSoftness = 0.44;
      float vignette = smoothstep(vigRadius, vigRadius - vigSoftness, dist);
      col *= vignette;

      gl_FragColor = vec4(col, 1.0);
    }
  `,
};

export class ValleyEngine {
  private container: HTMLElement;
  private renderer!: THREE.WebGLRenderer;
  private scene!: THREE.Scene;
  private camera!: THREE.PerspectiveCamera;
  private composer!: EffectComposer;
  private bloomPass!: UnrealBloomPass;
  private rgbNoisePass!: ShaderPass;
  private pathCurve!: THREE.CatmullRomCurve3;

  private flowerGeometry!: THREE.BufferGeometry;
  private flowerMaterial!: THREE.ShaderMaterial;
  private flowerPoints!: THREE.Points;

  private dustGeometry!: THREE.BufferGeometry;
  private dustMaterial!: THREE.ShaderMaterial;
  private dustPoints!: THREE.Points;

  private lightRayMaterial?: THREE.ShaderMaterial;

  private animFrameId: number = 0;
  private startTime = 0;
  private lastTime = 0;

  // Virtual scroller physics & navigation
  private progress = 0.0;
  private targetProgress = 0.0;
  private scrollVelocity = 0;
  private idleSpeed = 0.00032;

  private mouse = new THREE.Vector2(0, 0);
  private targetMouse = new THREE.Vector2(0, 0);
  private mouseRepel = new THREE.Vector2(-999, -999);

  // Hover transition
  private hoverProgress = 0;
  private isHovered = false;

  private introFinished = false;
  private onLoaded?: () => void;
  private onIntroDone?: () => void;

  constructor(container: HTMLElement, onLoaded?: () => void, onIntroDone?: () => void) {
    this.container = container;
    this.onLoaded = onLoaded;
    this.onIntroDone = onIntroDone;

    this.initScene();
    this.initPath();
    this.initFlowers();
    this.initDust();
    this.initLightRays();
    this.initPostProcessing();
    this.initEvents();

    this.startTime = typeof performance !== "undefined" ? performance.now() * 0.001 : 0;
    this.lastTime = this.startTime;

    setTimeout(() => {
      this.onLoaded?.();
      setTimeout(() => {
        this.introFinished = true;
        this.onIntroDone?.();
      }, 450);
    }, 300);

    this.animate();
  }

  private initScene() {
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x000000);
    this.scene.fog = new THREE.FogExp2(0x000000, 0.0068);

    const width = this.container.clientWidth || window.innerWidth;
    const height = this.container.clientHeight || window.innerHeight;

    this.camera = new THREE.PerspectiveCamera(54, width / height, 0.1, 1000);

    this.renderer = new THREE.WebGLRenderer({
      antialias: true,
      powerPreference: "high-performance",
      alpha: false,
    });
    this.renderer.setSize(width, height);
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.75;

    this.renderer.domElement.style.width = "100%";
    this.renderer.domElement.style.height = "100%";
    this.container.appendChild(this.renderer.domElement);
  }

  private initPostProcessing() {
    const width = this.container.clientWidth || window.innerWidth;
    const height = this.container.clientHeight || window.innerHeight;

    this.composer = new EffectComposer(this.renderer);
    this.composer.addPass(new RenderPass(this.scene, this.camera));

    // Ethereal floral bloom (luminous flower petals and dust embers)
    this.bloomPass = new UnrealBloomPass(
      new THREE.Vector2(width, height),
      0.68, // strength
      0.65, // radius
      0.16  // threshold
    );
    this.composer.addPass(this.bloomPass);

    // Split-RGB chromatic dispersion, film noise & dynamic velocity vignette
    this.rgbNoisePass = new ShaderPass(SplitRGBNoiseShader);
    this.rgbNoisePass.uniforms.uResolution.value.set(width, height);
    this.composer.addPass(this.rgbNoisePass);
  }

  private initPath() {
    // 3D valley flight path through the blooming garden
    const points: THREE.Vector3[] = [
      new THREE.Vector3(0, 2.5, 0),
      new THREE.Vector3(14, 2.8, -80),
      new THREE.Vector3(6, 3.2, -180),
      new THREE.Vector3(-10, 2.6, -290),
      new THREE.Vector3(-18, 3.1, -400),
      new THREE.Vector3(-4, 2.7, -510),
      new THREE.Vector3(16, 3.4, -620),
      new THREE.Vector3(10, 2.8, -730),
      new THREE.Vector3(-8, 3.0, -840),
      new THREE.Vector3(-15, 3.4, -950),
      new THREE.Vector3(3, 2.7, -1060),
      new THREE.Vector3(14, 3.0, -1170),
      new THREE.Vector3(0, 2.5, -1280),
    ];

    this.pathCurve = new THREE.CatmullRomCurve3(points, true, "centripetal");
  }

  /**
   * Generates a 1024x1024 High-Definition Botanical Atlas (3x2 Grid = 6 Distinct Species):
   * Row 0:
   *   Tile 0: Magenta/Pink Cosmos (layered notched petals with glowing yellow stamen center)
   *   Tile 1: Fiery Orange Poppy & Marigold (ruffled overlapping petals, deep vermilion to marigold gold)
   *   Tile 2: Lilac / Violet Wildflower (star/bell florets with radiant violet gradient and bright core)
   * Row 1:
   *   Tile 3: Golden Yellow Buttercup & Daisy (12 radiating slender golden petals, velvet disc)
   *   Tile 4: Botanical Fern Fronds & Pinnate Leaf (rachis stem with serrated green leaflets)
   *   Tile 5: Dense Wild Leaf Sprig & Ground Foliage (broad lanceolate leaves and clover grass tufts)
   */
  private createFloralAtlas(): THREE.Texture {
    const canvas = document.createElement("canvas");
    canvas.width = 1024;
    canvas.height = 1024;
    const ctx = canvas.getContext("2d")!;

    ctx.clearRect(0, 0, 1024, 1024);

    const tileW = 1024 / 3; // ~341.33 px
    const tileH = 512;

    const getCellCenter = (col: number, row: number) => ({
      cx: col * tileW + tileW / 2,
      cy: row * tileH + tileH / 2,
    });

    // =========================================================================
    // TILE 0: Magenta / Pink Cosmos Flower (Col 0, Row 0)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(0, 0);

      // Outer ethereal pink aura
      const aura = ctx.createRadialGradient(cx, cy, 10, cx, cy, 155);
      aura.addColorStop(0, "rgba(255, 120, 190, 0.5)");
      aura.addColorStop(0.65, "rgba(220, 30, 130, 0.18)");
      aura.addColorStop(1, "rgba(0, 0, 0, 0)");
      ctx.fillStyle = aura;
      ctx.beginPath();
      ctx.arc(cx, cy, 155, 0, Math.PI * 2);
      ctx.fill();

      // 8 overlapping velvety cosmos petals with fine notched edges
      for (let i = 0; i < 8; i++) {
        const ang = (i * Math.PI * 2) / 8;
        const px = cx + Math.cos(ang) * 58;
        const py = cy + Math.sin(ang) * 58;
        const pGrad = ctx.createRadialGradient(px, py, 4, px, py, 75);
        pGrad.addColorStop(0, "rgba(255, 235, 248, 1.0)");
        pGrad.addColorStop(0.35, "rgba(245, 50, 145, 0.96)");
        pGrad.addColorStop(0.72, "rgba(180, 15, 95, 0.85)");
        pGrad.addColorStop(0.96, "rgba(120, 5, 60, 0.35)");
        pGrad.addColorStop(1, "rgba(0, 0, 0, 0)");
        ctx.fillStyle = pGrad;
        ctx.beginPath();
        ctx.arc(px, py, 70, 0, Math.PI * 2);
        ctx.fill();
      }

      // Golden pollen stamen core
      const core = ctx.createRadialGradient(cx, cy, 0, cx, cy, 38);
      core.addColorStop(0, "rgba(255, 255, 255, 1.0)");
      core.addColorStop(0.4, "rgba(255, 235, 60, 1.0)");
      core.addColorStop(0.75, "rgba(235, 145, 20, 0.88)");
      core.addColorStop(1, "rgba(200, 90, 10, 0)");
      ctx.fillStyle = core;
      ctx.beginPath();
      ctx.arc(cx, cy, 38, 0, Math.PI * 2);
      ctx.fill();
    }

    // =========================================================================
    // TILE 1: Fiery Orange Marigold / Poppy (Col 1, Row 0)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(1, 0);

      // Warm golden-orange bloom aura
      const aura = ctx.createRadialGradient(cx, cy, 10, cx, cy, 155);
      aura.addColorStop(0, "rgba(255, 175, 45, 0.52)");
      aura.addColorStop(0.65, "rgba(240, 85, 10, 0.22)");
      aura.addColorStop(1, "rgba(0, 0, 0, 0)");
      ctx.fillStyle = aura;
      ctx.beginPath();
      ctx.arc(cx, cy, 155, 0, Math.PI * 2);
      ctx.fill();

      // 10 ruffled dense poppy petals
      for (let i = 0; i < 10; i++) {
        const ang = (i * Math.PI * 2) / 10 + 0.15;
        const px = cx + Math.cos(ang) * 56;
        const py = cy + Math.sin(ang) * 56;
        const pGrad = ctx.createRadialGradient(px, py, 4, px, py, 72);
        pGrad.addColorStop(0, "rgba(255, 245, 135, 1.0)");
        pGrad.addColorStop(0.4, "rgba(255, 125, 20, 0.96)");
        pGrad.addColorStop(0.75, "rgba(215, 45, 5, 0.88)");
        pGrad.addColorStop(0.96, "rgba(150, 25, 0, 0.35)");
        pGrad.addColorStop(1, "rgba(0, 0, 0, 0)");
        ctx.fillStyle = pGrad;
        ctx.beginPath();
        ctx.arc(px, py, 68, 0, Math.PI * 2);
        ctx.fill();
      }

      // Dark amber center seed disc
      const core = ctx.createRadialGradient(cx, cy, 0, cx, cy, 34);
      core.addColorStop(0, "rgba(255, 255, 190, 1.0)");
      core.addColorStop(0.45, "rgba(255, 165, 25, 0.98)");
      core.addColorStop(0.8, "rgba(150, 35, 0, 0.88)");
      core.addColorStop(1, "rgba(90, 15, 0, 0)");
      ctx.fillStyle = core;
      ctx.beginPath();
      ctx.arc(cx, cy, 34, 0, Math.PI * 2);
      ctx.fill();
    }

    // =========================================================================
    // TILE 2: Lilac / Violet Wildflower (Col 2, Row 0)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(2, 0);

      const aura = ctx.createRadialGradient(cx, cy, 10, cx, cy, 155);
      aura.addColorStop(0, "rgba(195, 140, 255, 0.5)");
      aura.addColorStop(0.65, "rgba(135, 65, 240, 0.2)");
      aura.addColorStop(1, "rgba(0, 0, 0, 0)");
      ctx.fillStyle = aura;
      ctx.beginPath();
      ctx.arc(cx, cy, 155, 0, Math.PI * 2);
      ctx.fill();

      // 6 star florets
      for (let i = 0; i < 6; i++) {
        const ang = (i * Math.PI * 2) / 6;
        const px = cx + Math.cos(ang) * 58;
        const py = cy + Math.sin(ang) * 58;
        const pGrad = ctx.createRadialGradient(px, py, 3, px, py, 66);
        pGrad.addColorStop(0, "rgba(245, 235, 255, 1.0)");
        pGrad.addColorStop(0.4, "rgba(185, 120, 255, 0.96)");
        pGrad.addColorStop(0.8, "rgba(115, 45, 210, 0.78)");
        pGrad.addColorStop(1, "rgba(0, 0, 0, 0)");
        ctx.fillStyle = pGrad;
        ctx.beginPath();
        ctx.arc(px, py, 62, 0, Math.PI * 2);
        ctx.fill();
      }

      // Ethereal white floral core
      const core = ctx.createRadialGradient(cx, cy, 0, cx, cy, 28);
      core.addColorStop(0, "rgba(255, 255, 255, 1.0)");
      core.addColorStop(0.4, "rgba(215, 235, 255, 0.96)");
      core.addColorStop(0.8, "rgba(150, 180, 255, 0.65)");
      core.addColorStop(1, "rgba(100, 120, 220, 0)");
      ctx.fillStyle = core;
      ctx.beginPath();
      ctx.arc(cx, cy, 28, 0, Math.PI * 2);
      ctx.fill();
    }

    // =========================================================================
    // TILE 3: Golden Yellow Buttercup & Daisy (Col 0, Row 1)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(0, 1);

      const aura = ctx.createRadialGradient(cx, cy, 10, cx, cy, 155);
      aura.addColorStop(0, "rgba(255, 245, 140, 0.5)");
      aura.addColorStop(0.65, "rgba(240, 190, 20, 0.22)");
      aura.addColorStop(1, "rgba(0, 0, 0, 0)");
      ctx.fillStyle = aura;
      ctx.beginPath();
      ctx.arc(cx, cy, 155, 0, Math.PI * 2);
      ctx.fill();

      // 12 radiating slender daisy petals
      for (let i = 0; i < 12; i++) {
        const ang = (i * Math.PI * 2) / 12;
        const px = cx + Math.cos(ang) * 62;
        const py = cy + Math.sin(ang) * 62;
        const pGrad = ctx.createRadialGradient(px, py, 3, px, py, 56);
        pGrad.addColorStop(0, "rgba(255, 255, 235, 1.0)");
        pGrad.addColorStop(0.35, "rgba(255, 225, 70, 0.96)");
        pGrad.addColorStop(0.75, "rgba(220, 160, 15, 0.78)");
        pGrad.addColorStop(1, "rgba(0, 0, 0, 0)");
        ctx.fillStyle = pGrad;
        ctx.beginPath();
        ctx.arc(px, py, 54, 0, Math.PI * 2);
        ctx.fill();
      }

      // Golden disc
      const core = ctx.createRadialGradient(cx, cy, 0, cx, cy, 34);
      core.addColorStop(0, "rgba(255, 255, 255, 1.0)");
      core.addColorStop(0.45, "rgba(255, 210, 25, 1.0)");
      core.addColorStop(0.8, "rgba(195, 125, 10, 0.82)");
      core.addColorStop(1, "rgba(140, 80, 0, 0)");
      ctx.fillStyle = core;
      ctx.beginPath();
      ctx.arc(cx, cy, 34, 0, Math.PI * 2);
      ctx.fill();
    }

    // =========================================================================
    // TILE 4: Botanical Fern Frond & Pointed Leaf (Col 1, Row 1)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(1, 1);

      ctx.save();
      ctx.translate(cx, cy);

      // Layered cluster of botanical pointed leaves
      const leafAngles = [-0.68, -0.24, 0.12, 0.48, 0.84];
      for (const ang of leafAngles) {
        ctx.save();
        ctx.rotate(ang);

        const leafGrad = ctx.createLinearGradient(0, -75, 0, 75);
        leafGrad.addColorStop(0, "rgba(125, 235, 140, 1.0)");
        leafGrad.addColorStop(0.4, "rgba(48, 170, 75, 0.96)");
        leafGrad.addColorStop(0.8, "rgba(20, 100, 40, 0.78)");
        leafGrad.addColorStop(1, "rgba(10, 45, 18, 0)");

        ctx.fillStyle = leafGrad;
        ctx.beginPath();
        ctx.moveTo(0, -80);
        ctx.bezierCurveTo(46, -38, 46, 38, 0, 80);
        ctx.bezierCurveTo(-46, 38, -46, -38, 0, -80);
        ctx.fill();

        // Central vein
        ctx.strokeStyle = "rgba(185, 255, 200, 0.85)";
        ctx.lineWidth = 3.2;
        ctx.beginPath();
        ctx.moveTo(0, -70);
        ctx.lineTo(0, 65);
        ctx.stroke();

        ctx.restore();
      }

      ctx.restore();
    }

    // =========================================================================
    // TILE 5: Dense Wild Leaf Sprig & Grass Tuft (Col 2, Row 1)
    // =========================================================================
    {
      const { cx, cy } = getCellCenter(2, 1);

      ctx.save();
      ctx.translate(cx, cy);

      // Dense overlapping grass blades and ground cover foliage
      for (let j = 0; j < 9; j++) {
        const ang = (j * Math.PI) / 4.5 - Math.PI / 2;
        ctx.save();
        ctx.rotate(ang);

        const bladeGrad = ctx.createLinearGradient(0, -95, 0, 10);
        bladeGrad.addColorStop(0, "rgba(140, 240, 155, 0.98)");
        bladeGrad.addColorStop(0.5, "rgba(38, 155, 65, 0.92)");
        bladeGrad.addColorStop(0.9, "rgba(16, 80, 30, 0.65)");
        bladeGrad.addColorStop(1, "rgba(0, 0, 0, 0)");

        ctx.fillStyle = bladeGrad;
        ctx.beginPath();
        ctx.moveTo(0, -95);
        ctx.quadraticCurveTo(19, -42, 6, 10);
        ctx.lineTo(-6, 10);
        ctx.quadraticCurveTo(-19, -42, 0, -95);
        ctx.fill();

        ctx.restore();
      }

      ctx.restore();
    }

    const texture = new THREE.CanvasTexture(canvas);
    texture.minFilter = THREE.LinearMipMapLinearFilter;
    texture.magFilter = THREE.LinearFilter;
    return texture;
  }

  private initFlowers() {
    // 52,000 botanical points forming the full lush valley garden
    const count = 52000;
    const positions = new Float32Array(count * 3);
    const dispersionOffsets = new Float32Array(count * 3);
    const colors = new Float32Array(count * 3);
    const scales = new Float32Array(count);
    const phases = new Float32Array(count);
    const spriteTypes = new Float32Array(count);

    // Primary palette
    const colOrange = new THREE.Color(0xff8c1a);
    const colPink = new THREE.Color(0xff459a);
    const colPurple = new THREE.Color(0xb388ff);
    const colYellow = new THREE.Color(0xffd500);
    const colGreen = new THREE.Color(0x38b000);
    const colDeepGreen = new THREE.Color(0x195b24);

    for (let i = 0; i < count; i++) {
      const idx = i * 3;
      const t = Math.random();
      const pathPoint = this.pathCurve.getPointAt(t);

      // Lateral distribution across valley
      const side = Math.random() > 0.5 ? 1 : -1;
      const u = Math.random();
      const dist = Math.pow(u, 0.6) * 52;

      const x = pathPoint.x + side * dist;
      const z = pathPoint.z + (Math.random() - 0.5) * 18;

      // Terrain elevation: valley glen in the middle with banks climbing on both sides
      const normDist = dist / 52;
      const bankClimb = Math.pow(normDist, 1.7) * 13.0;
      const wave = Math.sin(x * 0.08) * 1.35 + Math.cos(z * 0.06) * 1.05;
      const y = wave + bankClimb - 1.25 + (Math.random() - 0.5) * 0.6;

      positions[idx] = x;
      positions[idx + 1] = y;
      positions[idx + 2] = z;

      // Hover dispersion offsets (Image 1: flowers scatter into floating stardust embers)
      const dispRadius = 32 + Math.random() * 65;
      const dispAng = Math.random() * Math.PI * 2;
      dispersionOffsets[idx] = Math.cos(dispAng) * dispRadius;
      dispersionOffsets[idx + 1] = 20 + Math.random() * 48;
      dispersionOffsets[idx + 2] = Math.sin(dispAng) * dispRadius;

      // =========================================================================
      // ZONED BOTANICAL GARDEN DISTRIBUTION:
      // - 46% of total elements are lush green leaves & foliage grounding the flowers
      // - Right bank: dense field of Fiery Orange Poppies & Marigolds (Tile 1)
      // - Left bank: dense field of Magenta & Pink Cosmos (Tile 0)
      // - Center glen: Violet/Lilac Wildflowers (Tile 2) & Golden Buttercups (Tile 3)
      // =========================================================================
      let sType = 0;
      let col = colPink;
      const rChance = Math.random();

      if (rChance < 0.46) {
        // Lush botanical foliage
        if (Math.random() > 0.5) {
          sType = 4; // Fern / Pointed Leaf
          col = colGreen.clone().multiplyScalar(0.92 + Math.random() * 0.28);
        } else {
          sType = 5; // Dense Grass & Leaf Sprig
          col = colDeepGreen.clone().multiplyScalar(0.95 + Math.random() * 0.32);
        }
      } else {
        // Floral blossoms
        if (side > 0 && dist > 4.5) {
          // RIGHT BANK: Orange Poppies & Marigolds
          sType = 1;
          col = colOrange.clone().multiplyScalar(0.96 + Math.random() * 0.36);
        } else if (side < 0 && dist > 4.5) {
          // LEFT BANK: Pink & Magenta Cosmos
          sType = 0;
          col = colPink.clone().multiplyScalar(0.96 + Math.random() * 0.36);
        } else {
          // CENTER GLEN: Wildflowers
          if (Math.random() > 0.48) {
            sType = 2; // Purple / Lilac Wildflower
            col = colPurple.clone().multiplyScalar(0.96 + Math.random() * 0.32);
          } else {
            sType = 3; // Golden Buttercup
            col = colYellow.clone().multiplyScalar(0.96 + Math.random() * 0.32);
          }
        }
      }

      spriteTypes[i] = sType;
      colors[idx] = col.r;
      colors[idx + 1] = col.g;
      colors[idx + 2] = col.b;

      scales[i] = 0.95 + Math.random() * 1.4;
      phases[i] = Math.random() * Math.PI * 2;
    }

    this.flowerGeometry = new THREE.BufferGeometry();
    this.flowerGeometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    this.flowerGeometry.setAttribute("dispersionOffset", new THREE.BufferAttribute(dispersionOffsets, 3));
    this.flowerGeometry.setAttribute("color", new THREE.BufferAttribute(colors, 3));
    this.flowerGeometry.setAttribute("scale", new THREE.BufferAttribute(scales, 1));
    this.flowerGeometry.setAttribute("phase", new THREE.BufferAttribute(phases, 1));
    this.flowerGeometry.setAttribute("spriteType", new THREE.BufferAttribute(spriteTypes, 1));

    const atlasTexture = this.createFloralAtlas();

    this.flowerMaterial = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
        uTexture: { value: atlasTexture },
        uMouseRepel: { value: new THREE.Vector2(-999, -999) },
        uHover: { value: 0.0 },
      },
      vertexShader: `
        uniform float uTime;
        uniform vec2 uMouseRepel;
        uniform float uHover;

        attribute vec3 dispersionOffset;
        attribute vec3 color;
        attribute float scale;
        attribute float phase;
        attribute float spriteType;

        varying vec3 vColor;
        varying float vAlpha;
        varying float vSpriteType;

        void main() {
          vColor = color;
          vSpriteType = spriteType;

          // Base position combined with hover dispersion
          vec3 pos = position + dispersionOffset * uHover;

          // Organic gentle night breeze
          pos.y += sin(uTime * 1.6 + phase) * 0.18;
          pos.x += cos(uTime * 1.2 + pos.z * 0.06) * 0.14;

          vec4 mvPosition = modelViewMatrix * vec4(pos, 1.0);
          float dist = -mvPosition.z;

          // Screen-space mouse pushaway and cursor glow (Prompt requirement)
          vec4 clipPos = projectionMatrix * mvPosition;
          vec2 screenPos = clipPos.xy / clipPos.w;
          float mouseDist = length(screenPos - uMouseRepel);

          if (mouseDist < 0.46) {
            float repelFactor = smoothstep(0.46, 0.0, mouseDist);
            vec3 repelDir = normalize(vec3(screenPos - uMouseRepel, 0.35));
            pos += repelDir * repelFactor * 8.0;
            // Points nearest cursor glow intensely
            vColor += vec3(0.65, 0.8, 1.0) * repelFactor * 2.2;
          }

          mvPosition = modelViewMatrix * vec4(pos, 1.0);
          dist = -mvPosition.z;

          // Exact prompt behavior:
          // "Flowers bloom from small to full size as they approach the camera, and leaves grow in further back."
          bool isLeaf = spriteType >= 3.5;
          float growStart = isLeaf ? 145.0 : 70.0;
          float growEnd   = isLeaf ? 32.0 : 7.5;
          float growth    = smoothstep(growStart, growEnd, dist);

          float baseScale = scale * (isLeaf ? 1400.0 : 1750.0) / (dist + 4.8);
          float pointSize = baseScale * (0.15 + 0.85 * growth);

          if (uHover > 0.01) {
            pointSize *= (1.0 + uHover * 0.4);
            // Floating stardust golden-pink color transition during hover
            vColor = mix(vColor, vec3(1.0, 0.84, 0.6), uHover * 0.85);
          }

          gl_PointSize = clamp(pointSize, 2.0, 195.0);
          gl_Position = projectionMatrix * mvPosition;

          // Depth fog attenuation fading smoothly to black in the distance
          float fogNear = 8.0;
          float fogFar = 190.0;
          float fogFactor = clamp((fogFar - dist) / (fogFar - fogNear), 0.0, 1.0);
          vAlpha = clamp(dist * 0.24, 0.0, 1.0) * fogFactor;
        }
      `,
      fragmentShader: `
        uniform sampler2D uTexture;
        uniform float uHover;

        varying vec3 vColor;
        varying float vAlpha;
        varying float vSpriteType;

        void main() {
          // 3 cols x 2 rows in atlas
          float col = 0.0;
          float row = 0.0;

          if (vSpriteType < 0.5) {
            col = 0.0; row = 0.0; // Tile 0: Pink Cosmos
          } else if (vSpriteType < 1.5) {
            col = 1.0; row = 0.0; // Tile 1: Orange Poppy
          } else if (vSpriteType < 2.5) {
            col = 2.0; row = 0.0; // Tile 2: Purple Wildflower
          } else if (vSpriteType < 3.5) {
            col = 0.0; row = 1.0; // Tile 3: Yellow Buttercup
          } else if (vSpriteType < 4.5) {
            col = 1.0; row = 1.0; // Tile 4: Fern Leaf
          } else {
            col = 2.0; row = 1.0; // Tile 5: Dense Foliage Sprig
          }

          vec2 uv = vec2(
            (col + gl_PointCoord.x) / 3.0,
            (row + gl_PointCoord.y) / 2.0
          );

          vec4 texColor = texture2D(uTexture, uv);
          if (texColor.a < 0.03) discard;

          vec3 finalColor = vColor * texColor.rgb * (1.35 + uHover * 0.4);
          float finalAlpha = texColor.a * vAlpha;

          gl_FragColor = vec4(finalColor, finalAlpha);
        }
      `,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
    });

    this.flowerPoints = new THREE.Points(this.flowerGeometry, this.flowerMaterial);
    this.scene.add(this.flowerPoints);
  }

  private initDust() {
    const dustCount = 2000;
    const dustPos = new Float32Array(dustCount * 3);
    const dustSpeeds = new Float32Array(dustCount);

    for (let i = 0; i < dustCount; i++) {
      const idx = i * 3;
      const t = Math.random();
      const pt = this.pathCurve.getPointAt(t);

      dustPos[idx] = pt.x + (Math.random() - 0.5) * 58;
      dustPos[idx + 1] = pt.y + (Math.random() - 0.5) * 22 + 2.8;
      dustPos[idx + 2] = pt.z + (Math.random() - 0.5) * 58;
      dustSpeeds[i] = 0.4 + Math.random() * 1.5;
    }

    this.dustGeometry = new THREE.BufferGeometry();
    this.dustGeometry.setAttribute("position", new THREE.BufferAttribute(dustPos, 3));
    this.dustGeometry.setAttribute("speed", new THREE.BufferAttribute(dustSpeeds, 1));

    this.dustMaterial = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
      },
      vertexShader: `
        uniform float uTime;
        attribute float speed;
        varying float vAlpha;

        void main() {
          vec3 pos = position;
          pos.y += sin(uTime * 0.5 * speed + pos.x) * 1.8;
          pos.x += cos(uTime * 0.35 * speed + pos.z) * 1.3;

          vec4 mvPosition = modelViewMatrix * vec4(pos, 1.0);
          float dist = -mvPosition.z;

          gl_PointSize = clamp(26.0 / dist, 1.5, 9.0);
          gl_Position = projectionMatrix * mvPosition;

          vAlpha = clamp(dist * 0.08, 0.0, 0.65) * clamp((165.0 - dist) / 120.0, 0.0, 1.0);
        }
      `,
      fragmentShader: `
        varying float vAlpha;
        void main() {
          float d = length(gl_PointCoord - vec2(0.5));
          if (d > 0.5) discard;
          float intensity = smoothstep(0.5, 0.0, d);
          gl_FragColor = vec4(0.96, 0.94, 0.89, intensity * vAlpha * 0.75);
        }
      `,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
    });

    this.dustPoints = new THREE.Points(this.dustGeometry, this.dustMaterial);
    this.scene.add(this.dustPoints);
  }

  /**
   * Volumetric light ray sprites (God Rays) streaming through the valley
   * as specified in the original Gladeye engine recreation prompt.
   */
  private initLightRays() {
    const rayGroup = new THREE.Group();
    const rayGeo = new THREE.PlaneGeometry(16, 120);

    const rayMat = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
        uColor: { value: new THREE.Color(0xfff5e0) },
      },
      vertexShader: `
        varying vec2 vUv;
        void main() {
          vUv = uv;
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }
      `,
      fragmentShader: `
        uniform float uTime;
        uniform vec3 uColor;
        varying vec2 vUv;

        void main() {
          float vertical = pow(1.0 - vUv.y, 1.4) * smoothstep(0.0, 0.22, vUv.y);
          float horizontal = 1.0 - pow(abs(vUv.x - 0.5) * 2.0, 2.0);
          float beam = vertical * horizontal;
          if (beam < 0.005) discard;

          float shimmer = 0.85 + 0.15 * sin(uTime * 0.85 + vUv.y * 3.5);
          gl_FragColor = vec4(uColor, beam * 0.085 * shimmer);
        }
      `,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      side: THREE.DoubleSide,
    });

    this.lightRayMaterial = rayMat;

    // Place angled volumetric beams along the valley path
    for (let i = 0; i < 8; i++) {
      const t = i / 8;
      const pt = this.pathCurve.getPointAt(t);
      const mesh = new THREE.Mesh(rayGeo, rayMat);
      mesh.position.set(pt.x + (Math.sin(i * 1.7) * 14), pt.y + 36, pt.z - 12);
      mesh.rotation.x = 0.35;
      mesh.rotation.y = 0.25;
      mesh.rotation.z = -0.45;
      rayGroup.add(mesh);
    }

    this.scene.add(rayGroup);
  }

  private initEvents() {
    window.addEventListener(
      "wheel",
      (e) => {
        const delta = e.deltaY * 0.000045;
        this.scrollVelocity += delta;
      },
      { passive: true }
    );

    let touchStartY = 0;
    window.addEventListener(
      "touchstart",
      (e) => {
        if (e.touches.length > 0) touchStartY = e.touches[0].clientY;
      },
      { passive: true }
    );

    window.addEventListener(
      "touchmove",
      (e) => {
        if (e.touches.length > 0) {
          const deltaY = touchStartY - e.touches[0].clientY;
          touchStartY = e.touches[0].clientY;
          // 20x touch multiplier as required by the prompt
          this.scrollVelocity += deltaY * 0.000045 * 20;
        }
      },
      { passive: true }
    );

    window.addEventListener("pointermove", (e) => {
      const ndcX = (e.clientX / window.innerWidth) * 2 - 1;
      const ndcY = -(e.clientY / window.innerHeight) * 2 + 1;
      this.targetMouse.set(ndcX, ndcY);
      this.mouseRepel.set(ndcX, ndcY);
    });

    window.addEventListener("resize", this.handleResize);
  }

  private handleResize = () => {
    const width = this.container.clientWidth || window.innerWidth;
    const height = this.container.clientHeight || window.innerHeight;
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
    this.composer?.setSize(width, height);
    if (this.bloomPass) {
      this.bloomPass.resolution.set(width, height);
    }
    if (this.rgbNoisePass) {
      this.rgbNoisePass.uniforms.uResolution.value.set(width, height);
    }
  };

  public startHoverTransition() {
    this.isHovered = true;
  }

  public stopHoverTransition() {
    this.isHovered = false;
  }

  private animate = () => {
    this.animFrameId = requestAnimationFrame(this.animate);

    const now = typeof performance !== "undefined" ? performance.now() * 0.001 : 0;
    const delta = Math.min(Math.max(now - this.lastTime, 0.001), 0.1);
    this.lastTime = now;
    const elapsedTime = now - this.startTime;

    const targetHover = this.isHovered ? 1.0 : 0.0;
    this.hoverProgress += (targetHover - this.hoverProgress) * Math.min(delta * 2.8, 1.0);

    // Hover eases flight to a stop (prompt requirement)
    const currentSpeedMultiplier = 1.0 - this.hoverProgress * 0.96;
    this.scrollVelocity *= 0.92;
    this.targetProgress += (this.idleSpeed + this.scrollVelocity) * currentSpeedMultiplier;

    if (this.targetProgress > 1.0) this.targetProgress -= 1.0;
    if (this.targetProgress < 0.0) this.targetProgress += 1.0;

    this.progress += (this.targetProgress - this.progress) * 0.08;
    if (this.progress > 1.0) this.progress -= 1.0;
    if (this.progress < 0.0) this.progress += 1.0;

    const camPos = this.pathCurve.getPointAt(this.progress);
    const lookAheadProgress = (this.progress + 0.024) % 1.0;
    const lookTarget = this.pathCurve.getPointAt(lookAheadProgress);

    // Camera tilts toward mouse (pitch, yaw and roll)
    this.mouse.lerp(this.targetMouse, 0.06);
    this.camera.position.copy(camPos);
    this.camera.position.x += this.mouse.x * 2.2;
    this.camera.position.y += this.mouse.y * 1.3;

    this.camera.lookAt(
      lookTarget.x + this.mouse.x * 3.4,
      lookTarget.y + this.mouse.y * 1.8 - 0.28,
      lookTarget.z
    );
    this.camera.rotation.z = -this.mouse.x * 0.035;

    if (this.flowerMaterial) {
      this.flowerMaterial.uniforms.uTime.value = elapsedTime;
      this.flowerMaterial.uniforms.uMouseRepel.value.copy(this.mouseRepel);
      this.flowerMaterial.uniforms.uHover.value = this.hoverProgress;
    }

    if (this.dustMaterial) {
      this.dustMaterial.uniforms.uTime.value = elapsedTime;
    }

    if (this.lightRayMaterial) {
      this.lightRayMaterial.uniforms.uTime.value = elapsedTime;
    }

    // Update Split-RGB noise and dynamic velocity vignette in postprocessing
    if (this.rgbNoisePass) {
      this.rgbNoisePass.uniforms.uTime.value = elapsedTime;
      this.rgbNoisePass.uniforms.uVelocity.value = Math.abs(this.scrollVelocity) * 35.0;
    }

    // Render through full post-processing pipeline
    if (this.composer) {
      this.composer.render();
    } else {
      this.renderer.render(this.scene, this.camera);
    }

    if (this.introFinished && this.onIntroDone) {
      this.onIntroDone();
    }
  };

  public destroy() {
    cancelAnimationFrame(this.animFrameId);
    window.removeEventListener("resize", this.handleResize);

    this.flowerGeometry?.dispose();
    this.flowerMaterial?.dispose();
    this.dustGeometry?.dispose();
    this.dustMaterial?.dispose();
    this.lightRayMaterial?.dispose();
    this.composer?.dispose();
    this.renderer?.dispose();

    if (this.renderer?.domElement?.parentElement) {
      this.renderer.domElement.parentElement.removeChild(this.renderer.domElement);
    }
  }
}

export class ValleyAudioStore {
  private ctx: AudioContext | null = null;
  private isMuted: boolean = true;
  public isInitialized: boolean = false;

  private masterGain: GainNode | null = null;
  private filter: BiquadFilterNode | null = null;
  private oscillators: OscillatorNode[] = [];

  public initAudio() {
    if (this.isInitialized) return;

    try {
      const AudioCtx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
      this.ctx = new AudioCtx();

      this.masterGain = this.ctx.createGain();
      this.masterGain.gain.setValueAtTime(0, this.ctx.currentTime);

      this.filter = this.ctx.createBiquadFilter();
      this.filter.type = "lowpass";
      this.filter.frequency.setValueAtTime(14000, this.ctx.currentTime);
      this.filter.Q.setValueAtTime(1.2, this.ctx.currentTime);

      // Looping ambient drone chord (root 55Hz, harmonics, ethereal 5ths and octaves)
      const droneFreqs = [55, 110, 164.8, 220, 277.2, 440];
      droneFreqs.forEach((freq, idx) => {
        const osc = this.ctx!.createOscillator();
        osc.type = idx === 0 ? "sine" : "triangle";
        osc.frequency.setValueAtTime(freq, this.ctx!.currentTime);

        const oscGain = this.ctx!.createGain();
        const baseVol = 0.08 / (idx + 1);
        oscGain.gain.setValueAtTime(baseVol, this.ctx!.currentTime);

        osc.connect(oscGain);
        oscGain.connect(this.filter!);
        osc.start();
        this.oscillators.push(osc);
      });

      this.filter.connect(this.masterGain);
      this.masterGain.connect(this.ctx.destination);

      this.isInitialized = true;
    } catch {
      // Audio not supported
    }
  }

  public toggleMute(): boolean {
    if (!this.ctx || !this.masterGain) return true;

    if (this.ctx.state === "suspended") {
      void this.ctx.resume();
    }

    this.isMuted = !this.isMuted;
    const targetGain = this.isMuted ? 0 : 0.38;

    this.masterGain.gain.cancelScheduledValues(this.ctx.currentTime);
    this.masterGain.gain.linearRampToValueAtTime(targetGain, this.ctx.currentTime + 0.8);

    return this.isMuted;
  }

  public setHoverMuffle(isHovered: boolean) {
    if (!this.ctx || !this.filter) return;

    // Hover muffles the ambient sound through a low-pass filter (prompt requirement)
    const targetCutoff = isHovered ? 420 : 14000;
    this.filter.frequency.cancelScheduledValues(this.ctx.currentTime);
    this.filter.frequency.exponentialRampToValueAtTime(Math.max(targetCutoff, 100), this.ctx.currentTime + 0.9);
  }

  public destroy() {
    this.oscillators.forEach((o) => {
      try {
        o.stop();
        o.disconnect();
      } catch {}
    });
    this.oscillators = [];
    if (this.ctx) {
      void this.ctx.close();
      this.ctx = null;
    }
  }
}
