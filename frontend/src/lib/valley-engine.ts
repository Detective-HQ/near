import * as THREE from "three";
import { EffectComposer } from "three/examples/jsm/postprocessing/EffectComposer.js";
import { RenderPass } from "three/examples/jsm/postprocessing/RenderPass.js";
import { UnrealBloomPass } from "three/examples/jsm/postprocessing/UnrealBloomPass.js";
import { ShaderPass } from "three/examples/jsm/postprocessing/ShaderPass.js";
import { OutputPass } from "three/examples/jsm/postprocessing/OutputPass.js";

/*
 * Night flower valley.
 *
 * The field is one tile of W x D world units that repeats forever. Every
 * function that shapes it (terrain, the winding clearing, colour zones) is
 * periodic over the tile, so points can be wrapped around the camera in the
 * vertex shader and the flight never ends or shows a seam.
 */

const W = 120; // tile width (x)
const D = 96; // tile depth (z)
const AHEAD = 74; // how far in front of the camera the wrap window reaches
const TAU = Math.PI * 2;

const GLSL_FIELD = /* glsl */ `
  #define TAU 6.28318530718
  const float W = ${W.toFixed(1)};
  const float D = ${D.toFixed(1)};
  float pathX(float z) {
    return 7.0 * sin(TAU * z / D) + 3.0 * sin(TAU * 2.0 * z / D + 1.0);
  }
  float wrapDx(float x, float z) {
    return mod(x - pathX(z) + W * 0.5, W) - W * 0.5;
  }
  float terrain(float x, float z) {
    float dx = wrapDx(x, z);
    float h = 0.9 * sin(TAU * 2.0 * x / W + 1.3) * cos(TAU * z / D)
            + 0.6 * sin(TAU * (3.0 * z / D + x / W));
    return h + min(dx * dx * 0.0042, 5.5);
  }
`;

function pathX(z: number) {
  return 7 * Math.sin((TAU * z) / D) + 3 * Math.sin((TAU * 2 * z) / D + 1);
}
function wrapDx(x: number, z: number) {
  const v = x - pathX(z) + W * 0.5;
  return v - W * Math.floor(v / W) - W * 0.5;
}
function terrain(x: number, z: number) {
  const dx = wrapDx(x, z);
  const h =
    0.9 * Math.sin((TAU * 2 * x) / W + 1.3) * Math.cos((TAU * z) / D) +
    0.6 * Math.sin(TAU * ((3 * z) / D + x / W));
  return h + Math.min(dx * dx * 0.0042, 5.5);
}
const smooth = (a: number, b: number, v: number) => {
  const t = Math.min(Math.max((v - a) / (b - a), 0), 1);
  return t * t * (3 - 2 * t);
};

// Periodic noise-like fields that pick where each colour of flower grows.
function zoneA(x: number, z: number) {
  const u = x / W;
  const v = z / D;
  return (
    Math.sin(TAU * (2 * u + v) + 0.7) +
    0.6 * Math.sin(TAU * (3 * u - 2 * v) + 2.1) +
    0.4 * Math.sin(TAU * (5 * u + 4 * v) + 4.0)
  );
}
function zoneB(x: number, z: number) {
  const u = x / W;
  const v = z / D;
  return (
    Math.sin(TAU * (u - 3 * v) + 1.9) +
    0.7 * Math.sin(TAU * (4 * u + 2 * v) + 0.3) +
    0.35 * Math.sin(TAU * (6 * u - 5 * v) + 5.1)
  );
}
function density(x: number, z: number) {
  const u = x / W;
  const v = z / D;
  return (
    0.5 +
    0.3 * Math.sin(TAU * (3 * u + 2 * v) + 2.4) +
    0.25 * Math.sin(TAU * (5 * u - 3 * v) + 0.8) +
    0.15 * Math.sin(TAU * (9 * u + 7 * v) + 3.3)
  );
}

// Sprite atlas: 4 x 2 tiles.
const enum Sprite {
  Cosmos = 0,
  Poppy = 1,
  Violet = 2,
  Daisy = 3,
  Star = 4,
  Rose = 5,
  Leaf = 6,
  Sprig = 7,
}

const PostShader = {
  name: "ValleyGrade",
  uniforms: {
    tDiffuse: { value: null },
    uTime: { value: 0 },
    uVelocity: { value: 0 },
    uResolution: { value: new THREE.Vector2(1600, 900) },
    uBlur: { value: 14 },
  },
  vertexShader: /* glsl */ `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
    }
  `,
  fragmentShader: /* glsl */ `
    uniform sampler2D tDiffuse;
    uniform float uTime;
    uniform float uVelocity;
    uniform vec2 uResolution;
    uniform float uBlur;
    varying vec2 vUv;

    float hash(vec2 p) {
      vec3 p3 = fract(vec3(p.xyx) * 0.1031);
      p3 += dot(p3, p3.yzx + 33.33);
      return fract((p3.x + p3.y) * p3.z);
    }

    void main() {
      vec2 uv = vUv;
      vec2 c = uv - vec2(0.5, 0.53);
      float aspect = uResolution.x / uResolution.y;

      // Tilt-shift: sharp ellipse in the middle, soft bokeh toward the edges and the foreground.
      float e = length(c * vec2(1.0, 1.35));
      float blur = smoothstep(0.2, 0.62, e);
      blur = max(blur, smoothstep(0.36, 0.0, uv.y) * 0.85);
      blur = clamp(blur + uVelocity * 0.25, 0.0, 1.0);
      float radius = blur * uBlur;

      vec2 px = 1.0 / uResolution;
      float ab = (0.0016 + uVelocity * 0.006) * length(c);
      vec3 acc = vec3(0.0);
      float wsum = 0.0;
      // Golden-angle spiral samples; brighter samples weigh more so blooms turn into bokeh discs.
      for (int i = 0; i < 24; i++) {
        float fi = float(i);
        float r = sqrt((fi + 0.5) / 24.0) * radius;
        float a = fi * 2.39996323;
        vec2 o = vec2(cos(a), sin(a)) * r * px;
        vec2 suv = uv + o;
        vec3 s;
        s.r = texture2D(tDiffuse, suv + c * ab).r;
        s.g = texture2D(tDiffuse, suv).g;
        s.b = texture2D(tDiffuse, suv - c * ab).b;
        float w = 1.0 + dot(s, vec3(0.3, 0.5, 0.2)) * 2.5 * blur;
        acc += s * w;
        wsum += w;
      }
      vec3 col = acc / wsum;

      // Film grain
      vec2 g = uv * uResolution + vec2(sin(uTime * 37.0), cos(uTime * 43.0)) * 80.0;
      col += (hash(g) - 0.5) * 0.035;

      // Vignette that tightens as you fly faster
      float d = length(c * vec2(aspect * 0.62, 1.0));
      float vr = 0.95 - clamp(uVelocity * 1.6, 0.0, 0.3);
      col *= smoothstep(vr, vr - 0.62, d) * 0.92 + 0.08;
      // Night sky: the far end of the valley sinks into black.
      col *= mix(1.0, smoothstep(1.02, 0.7, uv.y), 0.9);

      gl_FragColor = vec4(max(col, 0.0), 1.0);
    }
  `,
};

export class ValleyEngine {
  private container: HTMLElement;
  private renderer?: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera: THREE.PerspectiveCamera;
  private composer?: EffectComposer;
  private bloomPass?: UnrealBloomPass;
  private gradePass?: ShaderPass;

  private fieldGeometry?: THREE.BufferGeometry;
  private fieldMaterial?: THREE.ShaderMaterial;
  private atlas?: THREE.Texture;
  private dustGeometry?: THREE.BufferGeometry;
  private dustMaterial?: THREE.ShaderMaterial;

  private animFrameId = 0;
  private startTime = 0;
  private lastTime = 0;
  private destroyed = false;
  private paused = false;

  // Flight along the clearing. camZ stays within one tile; the world repeats, so wrapping is invisible.
  private camZ = 0;
  private velocity = 0;
  private idleSpeed: number;

  private mouse = new THREE.Vector2(0, 0);
  private targetMouse = new THREE.Vector2(0, 0);
  private cursor = new THREE.Vector2(-9, -9);

  private hoverProgress = 0;
  private isHovered = false;

  private loadedSent = false;
  private introFinished = false;
  private onLoaded?: () => void;
  private onIntroDone?: () => void;

  private removeListeners: Array<() => void> = [];

  constructor(container: HTMLElement, onLoaded?: () => void, onIntroDone?: () => void) {
    this.container = container;
    this.onLoaded = onLoaded;
    this.onIntroDone = onIntroDone;

    const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
    this.idleSpeed = reduced ? 0.25 : 1.5;

    const { width, height } = this.size();
    this.camera = new THREE.PerspectiveCamera(48, width / height, 0.1, 400);

    try {
      this.renderer = new THREE.WebGLRenderer({ antialias: false, powerPreference: "high-performance" });
    } catch {
      // No WebGL: leave the black backdrop and let the interface appear.
      this.onLoaded?.();
      this.onIntroDone?.();
      return;
    }

    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
    this.renderer.setSize(width, height);
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 0.95;
    this.container.appendChild(this.renderer.domElement);

    this.scene.background = new THREE.Color(0x000000);

    const small = Math.min(width, height) < 700;
    this.initField(small ? 90000 : 200000);
    this.initDust(small ? 700 : 1400);
    this.initPost();
    this.initEvents();

    this.startTime = performance.now() * 0.001;
    this.lastTime = this.startTime;
    this.animate();
  }

  private size() {
    return {
      width: this.container.clientWidth || window.innerWidth,
      height: this.container.clientHeight || window.innerHeight,
    };
  }

  private projScale() {
    const h = this.renderer ? this.renderer.getDrawingBufferSize(new THREE.Vector2()).y : 900;
    return h / (2 * Math.tan(THREE.MathUtils.degToRad(this.camera.fov) / 2));
  }

  // ---------------------------------------------------------------------------
  // Sprites
  // ---------------------------------------------------------------------------

  private createAtlas(): THREE.Texture {
    const T = 256;
    const canvas = document.createElement("canvas");
    canvas.width = T * 4;
    canvas.height = T * 2;
    const ctx = canvas.getContext("2d")!;

    const tile = (i: number, draw: (cx: number, cy: number) => void) => {
      const cx = (i % 4) * T + T / 2;
      const cy = Math.floor(i / 4) * T + T / 2;
      ctx.save();
      ctx.beginPath();
      ctx.rect(cx - T / 2, cy - T / 2, T, T);
      ctx.clip();
      draw(cx, cy);
      ctx.restore();
    };

    const petals = (
      cx: number,
      cy: number,
      n: number,
      len: number,
      wid: number,
      inner: string,
      outer: string,
      rot = 0,
    ) => {
      for (let i = 0; i < n; i++) {
        const a = rot + (i / n) * TAU;
        ctx.save();
        ctx.translate(cx, cy);
        ctx.rotate(a);
        const g = ctx.createLinearGradient(0, 0, len, 0);
        g.addColorStop(0, inner);
        g.addColorStop(1, outer);
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.ellipse(len * 0.52, 0, len * 0.5, wid, 0, 0, TAU);
        ctx.fill();
        ctx.restore();
      }
    };
    const disc = (cx: number, cy: number, r: number, a: string, b: string) => {
      const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, r);
      g.addColorStop(0, a);
      g.addColorStop(1, b);
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.arc(cx, cy, r, 0, TAU);
      ctx.fill();
    };

    tile(Sprite.Cosmos, (cx, cy) => {
      petals(cx, cy, 8, 112, 34, "#ffd1ea", "#e8318a", 0.2);
      petals(cx, cy, 8, 84, 22, "#ffe6f3", "#f25aa5", 0.6);
      disc(cx, cy, 26, "#fff3a0", "#e0a020");
    });
    tile(Sprite.Poppy, (cx, cy) => {
      petals(cx, cy, 11, 112, 40, "#ffc36a", "#f0521a", 0);
      petals(cx, cy, 9, 78, 30, "#ffd690", "#ff6d22", 0.3);
      disc(cx, cy, 24, "#ffe9a8", "#9a3006");
    });
    tile(Sprite.Violet, (cx, cy) => {
      petals(cx, cy, 5, 112, 46, "#efe0ff", "#9258e8", -Math.PI / 2);
      disc(cx, cy, 22, "#ffffff", "#cdb8ff");
    });
    tile(Sprite.Daisy, (cx, cy) => {
      petals(cx, cy, 14, 112, 18, "#fff6c0", "#ffc61a", 0);
      disc(cx, cy, 30, "#ffe27a", "#c27a08");
    });
    tile(Sprite.Star, (cx, cy) => {
      petals(cx, cy, 5, 108, 40, "#ffffff", "#e8e2f6", -Math.PI / 2);
      disc(cx, cy, 18, "#fff3b0", "#e6c040");
    });
    tile(Sprite.Rose, (cx, cy) => {
      for (let r = 118; r > 18; r -= 20) {
        const t = r / 118;
        const n = 5 + Math.round(t * 4);
        petals(cx, cy, n, r, r * 0.42, "#fff1e8", t > 0.5 ? "#f59a80" : "#ffcbb8", r * 0.05);
      }
    });
    tile(Sprite.Leaf, (cx, cy) => {
      ctx.translate(cx, cy);
      const g = ctx.createLinearGradient(0, -120, 0, 120);
      g.addColorStop(0, "#d8f0c8");
      g.addColorStop(1, "#6e9a62");
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.moveTo(0, -122);
      ctx.bezierCurveTo(70, -50, 62, 60, 0, 122);
      ctx.bezierCurveTo(-62, 60, -70, -50, 0, -122);
      ctx.fill();
      ctx.strokeStyle = "rgba(40,70,40,0.55)";
      ctx.lineWidth = 5;
      ctx.beginPath();
      ctx.moveTo(0, -110);
      ctx.lineTo(0, 115);
      ctx.stroke();
    });
    tile(Sprite.Sprig, (cx, cy) => {
      ctx.translate(cx, cy);
      for (const a of [-0.9, -0.3, 0.3, 0.9, Math.PI]) {
        ctx.save();
        ctx.rotate(a);
        const g = ctx.createLinearGradient(0, 0, 0, -122);
        g.addColorStop(0, "#5d8a52");
        g.addColorStop(1, "#cbe8b6");
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.moveTo(0, 0);
        ctx.bezierCurveTo(36, -40, 28, -96, 0, -122);
        ctx.bezierCurveTo(-28, -96, -36, -40, 0, 0);
        ctx.fill();
        ctx.restore();
      }
    });

    const texture = new THREE.CanvasTexture(canvas);
    texture.colorSpace = THREE.SRGBColorSpace;
    texture.minFilter = THREE.LinearMipmapLinearFilter;
    texture.magFilter = THREE.LinearFilter;
    texture.generateMipmaps = true;
    texture.anisotropy = 4;
    return texture;
  }

  // ---------------------------------------------------------------------------
  // Field of flowers and leaves
  // ---------------------------------------------------------------------------

  private initField(count: number) {
    const positions = new Float32Array(count * 3);
    const tints = new Float32Array(count * 3);
    const sizes = new Float32Array(count);
    const kinds = new Float32Array(count);
    const seeds = new Float32Array(count);

    const tint = new THREE.Color();
    let n = 0;
    let guard = 0;
    while (n < count && guard < count * 6) {
      guard++;
      const x = Math.random() * W;
      const z = Math.random() * D;
      const dx = Math.abs(wrapDx(x, z));
      const clearing = smooth(1.2, 6.5, dx); // 0 in the path, 1 on the banks
      const dens = density(x, z);

      let kind: number;
      const flowerChance = Math.min(0.95, 0.1 + 0.85 * smooth(0.3, 0.68, dens)) * clearing;
      if (Math.random() < flowerChance) {
        const a = zoneA(x, z) + (Math.random() - 0.5) * 0.9;
        const b = zoneB(x, z) + (Math.random() - 0.5) * 0.9;
        if (Math.random() < 0.08) kind = Math.floor(Math.random() * 6);
        else if (a > 0.55) kind = Math.random() < 0.8 ? Sprite.Cosmos : Sprite.Star;
        else if (a < -0.55) kind = Math.random() < 0.72 ? Sprite.Poppy : Sprite.Rose;
        else if (b > 0.35) kind = Math.random() < 0.8 ? Sprite.Violet : Sprite.Star;
        else if (b < -0.35) kind = Math.random() < 0.75 ? Sprite.Daisy : Sprite.Poppy;
        else kind = [Sprite.Rose, Sprite.Star, Sprite.Violet, Sprite.Cosmos][Math.floor(Math.random() * 4)];
      } else {
        // Leaves fill the ground everywhere, sparser in the clearing.
        if (Math.random() > 0.35 + 0.65 * clearing && Math.random() < 0.55) continue;
        kind = Math.random() < 0.55 ? Sprite.Leaf : Sprite.Sprig;
      }

      const i3 = n * 3;
      positions[i3] = x;
      positions[i3 + 1] = Math.random();
      positions[i3 + 2] = z;

      if (kind >= Sprite.Leaf) {
        // Night greens: deep, slightly blue in shade, a few catching the moonlight.
        const light = 0.035 + Math.pow(Math.random(), 2.4) * 0.11;
        tint.setHSL(0.27 + Math.random() * 0.08, 0.45 + Math.random() * 0.25, light);
        sizes[n] = 0.3 + Math.random() * 0.4;
      } else {
        const v = 0.62 + Math.random() * 0.45;
        tint.setRGB(v, v * (0.92 + Math.random() * 0.12), v * (0.9 + Math.random() * 0.15));
        sizes[n] = 0.17 + Math.pow(Math.random(), 1.8) * 0.24;
      }
      tints[i3] = tint.r;
      tints[i3 + 1] = tint.g;
      tints[i3 + 2] = tint.b;
      kinds[n] = kind;
      seeds[n] = Math.random();
      n++;
    }

    const geo = new THREE.BufferGeometry();
    geo.setAttribute("position", new THREE.BufferAttribute(positions.subarray(0, n * 3), 3));
    geo.setAttribute("tint", new THREE.BufferAttribute(tints.subarray(0, n * 3), 3));
    geo.setAttribute("size", new THREE.BufferAttribute(sizes.subarray(0, n), 1));
    geo.setAttribute("kind", new THREE.BufferAttribute(kinds.subarray(0, n), 1));
    geo.setAttribute("seed", new THREE.BufferAttribute(seeds.subarray(0, n), 1));
    // Points move in the shader, so skip frustum culling of the static bounds.
    geo.boundingSphere = new THREE.Sphere(new THREE.Vector3(), 1e6);
    this.fieldGeometry = geo;

    this.atlas = this.createAtlas();

    this.fieldMaterial = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
        uAtlas: { value: this.atlas },
        uCam: { value: new THREE.Vector2() },
        uCursor: { value: new THREE.Vector2(-9, -9) },
        uAspect: { value: 1 },
        uHover: { value: 0 },
        uProj: { value: this.projScale() },
      },
      vertexShader: /* glsl */ `
        ${GLSL_FIELD}
        uniform float uTime;
        uniform vec2 uCam;
        uniform vec2 uCursor;
        uniform float uAspect;
        uniform float uHover;
        uniform float uProj;

        attribute vec3 tint;
        attribute float size;
        attribute float kind;
        attribute float seed;

        varying vec3 vTint;
        varying float vKind;
        varying float vRot;
        varying float vFog;
        varying float vGlow;
        varying float vHover;

        void main() {
          bool leaf = kind > 5.5;

          // Wrap the tile around the camera
          vec3 p = position;
          float x0 = uCam.x - W * 0.5;
          float z0 = uCam.y - ${AHEAD.toFixed(1)};
          p.x = x0 + mod(position.x - x0, W);
          p.z = z0 + mod(position.z - z0, D);
          float jitterY = position.y;
          p.y = terrain(p.x, p.z) + (leaf ? jitterY * 0.25 : 0.18 + jitterY * 0.35);

          // Night breeze
          float sway = sin(uTime * 1.3 + seed * 40.0 + p.z * 0.15);
          p.x += sway * (leaf ? 0.04 : 0.07);
          p.y += cos(uTime * 1.7 + seed * 23.0) * 0.03;

          // Hover: flowers lift off and drift apart into glowing seeds
          float hv = leaf ? 0.0 : uHover * smoothstep(0.0, 1.0, uHover * (0.6 + seed));
          vec2 away = normalize(p.xz - uCam + vec2(0.001));
          p.y += hv * (1.5 + seed * 7.0) + hv * sin(uTime * 0.8 + seed * 30.0) * 0.6;
          p.xz += away * hv * (1.0 + fract(seed * 7.3) * 5.0);
          vHover = hv;

          vec4 mv = modelViewMatrix * vec4(p, 1.0);
          float dist = -mv.z;
          vec4 clip = projectionMatrix * mv;

          // Cursor pushes points away in screen space; the closest ones glow.
          vec2 ndc = clip.xy / clip.w;
          vec2 dv = (ndc - uCursor) * vec2(uAspect, 1.0);
          float md = length(dv);
          float push = smoothstep(0.42, 0.0, md);
          vec2 dir = md > 0.0001 ? dv / md : vec2(0.0);
          ndc += dir / vec2(uAspect, 1.0) * push * push * 0.16;
          clip.xy = ndc * clip.w;
          vGlow = smoothstep(0.24, 0.0, md);

          // Flowers bloom as they come close; leaves grow in further back.
          float grow = leaf ? smoothstep(78.0, 34.0, dist) : smoothstep(46.0, 18.0, dist);
          float s = size * (0.35 + 0.65 * grow) * (1.0 + vGlow * 0.35);
          s *= mix(1.0, 0.3, hv);
          gl_PointSize = clamp(s * uProj / max(dist, 0.5), 0.0, 220.0);
          gl_Position = clip;

          vFog = exp(-pow(max(dist - 4.0, 0.0) / 27.0, 2.0));
          vTint = tint;
          vKind = kind;
          vRot = seed * 6.2831;
        }
      `,
      fragmentShader: /* glsl */ `
        uniform sampler2D uAtlas;
        uniform float uHover;

        varying vec3 vTint;
        varying float vKind;
        varying float vRot;
        varying float vFog;
        varying float vGlow;
        varying float vHover;

        void main() {
          vec2 pc = gl_PointCoord - 0.5;
          float c = cos(vRot), s = sin(vRot);
          pc = mat2(c, -s, s, c) * pc;
          if (max(abs(pc.x), abs(pc.y)) > 0.5) discard;

          float k = floor(vKind + 0.5);
          vec2 tile = vec2(mod(k, 4.0), floor(k / 4.0));
          vec2 uv = (tile + pc + 0.5) / vec2(4.0, 2.0);
          uv.y = 1.0 - uv.y;
          vec4 tex = texture2D(uAtlas, uv);

          // Seeds in flight become soft glowing dots
          float dotShape = smoothstep(0.5, 0.1, length(pc));
          float alpha = mix(tex.a, dotShape, vHover);
          if (alpha < 0.5) discard;

          bool leaf = vKind > 5.5;
          vec3 col = tex.rgb * vTint * (leaf ? 1.0 : 1.25);
          col = mix(col, max(col, vec3(0.1)) * 1.4 + vec3(0.9, 0.72, 0.45) * 0.55, vHover * 0.85);
          col += col * vGlow * 1.6 + vec3(0.9, 0.85, 0.7) * vGlow * 0.12;
          if (leaf) col *= 1.0 - uHover * 0.45;

          gl_FragColor = vec4(col * vFog, 1.0);
        }
      `,
      depthWrite: true,
      depthTest: true,
    });

    const points = new THREE.Points(geo, this.fieldMaterial);
    points.frustumCulled = false;
    this.scene.add(points);

    // Dark ground beneath the flowers so gaps read as soil, not void.
    const groundGeo = new THREE.PlaneGeometry(W * 1.6, D * 1.6, 96, 80);
    groundGeo.rotateX(-Math.PI / 2);
    const groundMat = new THREE.ShaderMaterial({
      uniforms: { uCam: this.fieldMaterial.uniforms.uCam },
      vertexShader: /* glsl */ `
        ${GLSL_FIELD}
        uniform vec2 uCam;
        varying float vFog;
        varying float vDx;
        void main() {
          vec3 p = position;
          p.x += uCam.x;
          p.z += uCam.y - D * 0.25;
          p.y = terrain(p.x, p.z) - 0.05;
          vDx = abs(wrapDx(p.x, p.z));
          vec4 mv = modelViewMatrix * vec4(p, 1.0);
          vFog = exp(-pow(max(-mv.z - 4.0, 0.0) / 27.0, 2.0));
          gl_Position = projectionMatrix * mv;
        }
      `,
      fragmentShader: /* glsl */ `
        varying float vFog;
        varying float vDx;
        void main() {
          vec3 soil = mix(vec3(0.012, 0.022, 0.012), vec3(0.02, 0.045, 0.02), smoothstep(2.0, 12.0, vDx));
          gl_FragColor = vec4(soil * vFog, 1.0);
        }
      `,
    });
    const ground = new THREE.Mesh(groundGeo, groundMat);
    ground.frustumCulled = false;
    this.scene.add(ground);
    this.disposables.push(groundGeo, groundMat);
  }

  private disposables: Array<{ dispose(): void }> = [];

  private initDust(count: number) {
    const pos = new Float32Array(count * 3);
    const seed = new Float32Array(count);
    for (let i = 0; i < count; i++) {
      pos[i * 3] = Math.random() * W;
      pos[i * 3 + 1] = 0.5 + Math.random() * 9;
      pos[i * 3 + 2] = Math.random() * D;
      seed[i] = Math.random();
    }
    this.dustGeometry = new THREE.BufferGeometry();
    this.dustGeometry.setAttribute("position", new THREE.BufferAttribute(pos, 3));
    this.dustGeometry.setAttribute("seed", new THREE.BufferAttribute(seed, 1));
    this.dustGeometry.boundingSphere = new THREE.Sphere(new THREE.Vector3(), 1e6);

    this.dustMaterial = new THREE.ShaderMaterial({
      uniforms: {
        uTime: { value: 0 },
        uCam: this.fieldMaterial!.uniforms.uCam,
        uProj: this.fieldMaterial!.uniforms.uProj,
        uHover: this.fieldMaterial!.uniforms.uHover,
      },
      vertexShader: /* glsl */ `
        ${GLSL_FIELD}
        uniform float uTime;
        uniform vec2 uCam;
        uniform float uProj;
        uniform float uHover;
        attribute float seed;
        varying float vAlpha;
        void main() {
          vec3 p = position;
          float x0 = uCam.x - W * 0.5;
          float z0 = uCam.y - ${AHEAD.toFixed(1)};
          p.x = x0 + mod(position.x - x0 + sin(uTime * 0.2 + seed * 9.0) * 2.0, W);
          p.z = z0 + mod(position.z - z0, D);
          p.y += terrain(p.x, p.z) + sin(uTime * (0.3 + seed * 0.4) + seed * 20.0) * 0.8;
          vec4 mv = modelViewMatrix * vec4(p, 1.0);
          float dist = -mv.z;
          gl_PointSize = clamp((0.05 + seed * 0.05) * uProj / dist, 1.0, 8.0);
          gl_Position = projectionMatrix * mv;
          float tw = 0.5 + 0.5 * sin(uTime * (1.0 + seed * 2.0) + seed * 50.0);
          vAlpha = tw * exp(-pow(dist / 40.0, 2.0)) * (0.55 + uHover * 0.6);
        }
      `,
      fragmentShader: /* glsl */ `
        varying float vAlpha;
        void main() {
          float d = length(gl_PointCoord - 0.5);
          float a = smoothstep(0.5, 0.0, d) * vAlpha;
          gl_FragColor = vec4(vec3(1.0, 0.93, 0.78) * a * 1.6, 1.0);
        }
      `,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
    });
    const dust = new THREE.Points(this.dustGeometry, this.dustMaterial);
    dust.frustumCulled = false;
    this.scene.add(dust);
  }

  private initPost() {
    if (!this.renderer) return;
    const { width, height } = this.size();
    this.composer = new EffectComposer(this.renderer);
    this.composer.addPass(new RenderPass(this.scene, this.camera));
    this.bloomPass = new UnrealBloomPass(new THREE.Vector2(width, height), 0.4, 0.5, 0.62);
    this.composer.addPass(this.bloomPass);
    this.gradePass = new ShaderPass(PostShader);
    this.composer.addPass(this.gradePass);
    this.composer.addPass(new OutputPass());
    this.updatePostSize();
  }

  private updatePostSize() {
    if (!this.renderer || !this.gradePass) return;
    const buf = this.renderer.getDrawingBufferSize(new THREE.Vector2());
    this.gradePass.uniforms.uResolution.value.copy(buf);
    this.gradePass.uniforms.uBlur.value = 13 * this.renderer.getPixelRatio();
    if (this.fieldMaterial) {
      this.fieldMaterial.uniforms.uProj.value = this.projScale();
      this.fieldMaterial.uniforms.uAspect.value = buf.x / buf.y;
    }
  }

  private initEvents() {
    const on = <K extends keyof WindowEventMap>(type: K, fn: (e: WindowEventMap[K]) => void) => {
      window.addEventListener(type, fn, { passive: true });
      this.removeListeners.push(() => window.removeEventListener(type, fn));
    };

    on("wheel", (e) => {
      this.velocity += e.deltaY * 0.012;
    });

    let touchY = 0;
    on("touchstart", (e) => {
      if (e.touches.length) touchY = e.touches[0].clientY;
    });
    on("touchmove", (e) => {
      if (!e.touches.length) return;
      const dy = touchY - e.touches[0].clientY;
      touchY = e.touches[0].clientY;
      this.velocity += dy * 0.012 * 2.4;
    });

    on("pointermove", (e) => {
      const r = this.container.getBoundingClientRect();
      const x = ((e.clientX - r.left) / (r.width || 1)) * 2 - 1;
      const y = -((e.clientY - r.top) / (r.height || 1)) * 2 + 1;
      this.targetMouse.set(x, y);
      this.cursor.set(x, y);
    });
    on("resize", this.handleResize);
  }

  private handleResize = () => {
    if (!this.renderer) return;
    const { width, height } = this.size();
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
    this.composer?.setSize(width, height);
    this.updatePostSize();
  };

  /** Stop rendering while the valley is scrolled out of view. */
  public setPaused(paused: boolean) {
    if (paused === this.paused || this.destroyed) return;
    this.paused = paused;
    if (paused) {
      cancelAnimationFrame(this.animFrameId);
    } else {
      this.lastTime = performance.now() * 0.001;
      this.animate();
    }
  }

  public startHoverTransition() {
    this.isHovered = true;
  }

  public stopHoverTransition() {
    this.isHovered = false;
  }

  private animate = () => {
    if (this.destroyed || this.paused || !this.renderer) return;
    this.animFrameId = requestAnimationFrame(this.animate);

    const now = performance.now() * 0.001;
    const dt = Math.min(Math.max(now - this.lastTime, 0.001), 0.1);
    this.lastTime = now;
    const t = now - this.startTime;

    this.hoverProgress += ((this.isHovered ? 1 : 0) - this.hoverProgress) * Math.min(dt * 2.2, 1);

    // Flight: idle drift plus scroll velocity, eased to a stop while hovering the arrow.
    this.velocity *= Math.pow(0.08, dt);
    this.velocity = THREE.MathUtils.clamp(this.velocity, -40, 60);
    const intro = Math.min(t / 3.6, 1);
    const introEase = 1 - Math.pow(1 - intro, 3);
    const speed = (this.idleSpeed * (0.3 + 0.7 * introEase) + this.velocity) * (1 - this.hoverProgress * 0.97);
    this.camZ -= speed * dt;
    if (this.camZ < -D) this.camZ += D;
    if (this.camZ > 0) this.camZ -= D;

    this.mouse.lerp(this.targetMouse, Math.min(dt * 3, 1));

    // Camera descends into the valley during the intro.
    const cz = this.camZ;
    const cx = pathX(cz) + this.mouse.x * 1.2;
    const lookZ = cz - 13;
    const lift = (1 - introEase) * 20;
    const cy = terrain(pathX(cz), cz) + 11 + lift;
    this.camera.position.set(cx, cy, cz + (1 - introEase) * 8);
    this.camera.lookAt(
      pathX(lookZ) + this.mouse.x * 2.6,
      terrain(pathX(lookZ), lookZ) - 2.2 + this.mouse.y * 1.6 - (1 - introEase) * 6,
      lookZ,
    );
    this.camera.rotateZ(-this.mouse.x * 0.03);

    const speedNorm = Math.min(Math.abs(this.velocity) / 30, 1);

    if (this.fieldMaterial) {
      const u = this.fieldMaterial.uniforms;
      u.uTime.value = t;
      u.uCam.value.set(cx, cz);
      u.uCursor.value.copy(this.cursor);
      u.uHover.value = this.hoverProgress;
    }
    if (this.dustMaterial) this.dustMaterial.uniforms.uTime.value = t;
    if (this.gradePass) {
      this.gradePass.uniforms.uTime.value = t;
      this.gradePass.uniforms.uVelocity.value = speedNorm;
    }
    if (this.bloomPass) this.bloomPass.strength = 0.4 + this.hoverProgress * 0.25;

    if (this.composer) this.composer.render(dt);
    else this.renderer.render(this.scene, this.camera);

    if (!this.loadedSent) {
      this.loadedSent = true;
      this.onLoaded?.();
    }
    if (!this.introFinished && intro > 0.7) {
      this.introFinished = true;
      this.onIntroDone?.();
    }
  };

  public destroy() {
    this.destroyed = true;
    cancelAnimationFrame(this.animFrameId);
    this.removeListeners.forEach((off) => off());
    this.removeListeners = [];

    this.fieldGeometry?.dispose();
    this.fieldMaterial?.dispose();
    this.atlas?.dispose();
    this.dustGeometry?.dispose();
    this.dustMaterial?.dispose();
    this.disposables.forEach((d) => d.dispose());
    this.composer?.dispose();
    this.renderer?.dispose();
    this.renderer?.domElement.remove();
  }
}

export class ValleyAudioStore {
  private ctx: AudioContext | null = null;
  private isMuted = true;
  public isInitialized = false;

  private masterGain: GainNode | null = null;
  private filter: BiquadFilterNode | null = null;
  private oscillators: OscillatorNode[] = [];

  public initAudio() {
    if (this.isInitialized) return;
    try {
      const AudioCtx =
        window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
      this.ctx = new AudioCtx();

      this.masterGain = this.ctx.createGain();
      this.masterGain.gain.setValueAtTime(0, this.ctx.currentTime);

      this.filter = this.ctx.createBiquadFilter();
      this.filter.type = "lowpass";
      this.filter.frequency.setValueAtTime(14000, this.ctx.currentTime);
      this.filter.Q.setValueAtTime(1.2, this.ctx.currentTime);

      // Soft ambient drone: root, fifth and octaves, slightly detuned.
      [55, 110, 164.8, 220, 277.2, 440].forEach((freq, idx) => {
        const osc = this.ctx!.createOscillator();
        osc.type = idx === 0 ? "sine" : "triangle";
        osc.frequency.setValueAtTime(freq, this.ctx!.currentTime);
        osc.detune.setValueAtTime((Math.random() - 0.5) * 8, this.ctx!.currentTime);
        const g = this.ctx!.createGain();
        g.gain.setValueAtTime(0.08 / (idx + 1), this.ctx!.currentTime);
        osc.connect(g);
        g.connect(this.filter!);
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
    if (this.ctx.state === "suspended") void this.ctx.resume();

    this.isMuted = !this.isMuted;
    const now = this.ctx.currentTime;
    this.masterGain.gain.cancelScheduledValues(now);
    this.masterGain.gain.setValueAtTime(this.masterGain.gain.value, now);
    this.masterGain.gain.linearRampToValueAtTime(this.isMuted ? 0 : 0.38, now + 0.8);
    return this.isMuted;
  }

  public setHoverMuffle(isHovered: boolean) {
    if (!this.ctx || !this.filter) return;
    const now = this.ctx.currentTime;
    this.filter.frequency.cancelScheduledValues(now);
    this.filter.frequency.setValueAtTime(this.filter.frequency.value, now);
    this.filter.frequency.exponentialRampToValueAtTime(isHovered ? 420 : 14000, now + 0.9);
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
