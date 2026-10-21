import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals, estimate } from '../testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from '../vitals.service';
import { TransparencyPanel } from './transparency-panel';

describe('TransparencyPanel', () => {
  let fake: FakeVitals;
  let el: HTMLElement;
  let whenStable: () => Promise<void>;

  beforeEach(async () => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    const service = TestBed.inject(VitalsService);
    await service.consentAndStart(document.createElement('video'));
    const fixture = TestBed.createComponent(TransparencyPanel);
    el = fixture.nativeElement as HTMLElement;
    whenStable = () => fixture.whenStable();
    await whenStable();
  });

  function toggle(): HTMLButtonElement {
    const button = el.querySelector('button');
    if (button === null) throw new Error('no toggle');
    return button;
  }

  async function open(): Promise<void> {
    toggle().click();
    await whenStable();
  }

  it('is collapsed until the toggle is pressed', async () => {
    expect(toggle().textContent.trim()).toBe('Show details');
    expect(el.querySelector('[data-testid="details"]')).toBeNull();

    await open();

    expect(toggle().textContent.trim()).toBe('Hide details');
    expect(el.querySelector('[data-testid="details"]')).not.toBeNull();
  });

  it('shows quality, confidence, method, frame cost and dropped frames', async () => {
    fake.emit('estimate', estimate({ quality: 0.8049, confidence: 'medium', method: 'pos' }));
    fake.emit('diagnostics', { frameCostMs: 12.34, droppedFrames: 3, pulse: [] });
    await open();
    const text = el.textContent.replace(/\s+/g, ' ');

    expect(text).toContain('Quality: 0.80');
    expect(text).toContain('Confidence: medium');
    expect(text).toContain('Method: pos');
    expect(text).toContain('Frame cost: 12.3 ms');
    expect(text).toContain('Dropped frames: 3');
  });

  it('draws the pulse as an SVG polyline with no canvas', async () => {
    fake.emit('diagnostics', { frameCostMs: 10, droppedFrames: 0, pulse: [0, 1, 0, -1] });
    await open();

    const line = el.querySelector('svg polyline');
    expect(el.querySelector('canvas')).toBeNull();
    expect(line?.getAttribute('points')).toBe('0,30 100,0 200,30 300,60');
  });

  it('draws a flat pulse in the middle', async () => {
    fake.emit('diagnostics', { frameCostMs: 10, droppedFrames: 0, pulse: [5, 5] });
    await open();

    expect(el.querySelector('polyline')?.getAttribute('points')).toBe('0,30 300,30');
  });

  it('draws nothing for an empty pulse', async () => {
    fake.emit('diagnostics', { frameCostMs: 10, droppedFrames: 0, pulse: [] });
    await open();

    expect(el.querySelector('polyline')).toBeNull();
  });
});
